#include "ble/StatusBleServer.h"

#include <cstring>

namespace {
// 断开连接后，每隔一段时间检查一次广播状态。
// 不需要每个 loop 都调用 startAdvertising()，否则会让日志和 BLE 状态变得很吵。
constexpr uint32_t kAdvertiseCheckIntervalMs = 1000;

// 桌面端每 10 秒发送一次 heartbeat。30 秒没有收到任何桌面端消息时，
// 认为桌面应用已停止同步；保留一定余量以容忍短暂卡顿或蓝牙调度延迟。
constexpr uint32_t kDesktopHeartbeatTimeoutMs = 30000;
}

void StatusBleServer::begin() {
  // 不把 Config() 写成 begin 的默认参数：当前 Arduino 工具链的 C++ 编译器
  // 对嵌套结构体成员默认值与默认参数的组合兼容性不足。
  begin(Config{});
}

void StatusBleServer::begin(const Config &config) {
  config_ = config;

  // 回调与业务解析之间使用固定长度队列。创建失败时仍允许 BLE 启动，但入站消息
  // 会被丢弃并在串口说明原因，便于定位极端内存不足问题。
  inboundQueue_ = xQueueCreate(kInboundQueueDepth, sizeof(InboundMessage));
  if (inboundQueue_ == nullptr) {
    Serial.println("BLE inbound queue allocation failed");
  }

  Serial.println("BLE init");
  Serial.print("  device: ");
  Serial.println(config_.deviceName);
  Serial.print("  service: ");
  Serial.println(config_.serviceUuid);
  Serial.print("  rx: ");
  Serial.println(config_.rxCharacteristicUuid);
  Serial.print("  tx: ");
  Serial.println(config_.txCharacteristicUuid);

  // 初始化 BLE 协议栈，并设置设备名。
  // 这个名字会出现在电脑端的蓝牙扫描结果里。
  NimBLEDevice::init(config_.deviceName);

  // 提高发射功率，让桌面场景下连接更稳定。
  // 如果后续做电池版本，可以考虑降低功率来省电。
  NimBLEDevice::setPower(ESP_PWR_LVL_P9);

  // 开启 bonding。这里没有屏幕输入配对码，也没有键盘输入能力，
  // 所以使用 NoInputNoOutput 的配对方式。
  //
  // 参数含义：
  // - bonding: 让电脑端和 ESP32-S3 可以保存绑定关系
  // - mitm: 此硬件没有键盘、确认键或配对码输入，必须关闭；若要求 MITM，macOS
  //   可能在首次 GATT Write 时等待一个永远无法完成的交互配对，最终表现为超时
  // - sc: 在设备能力允许时使用 Secure Connections
  NimBLEDevice::setSecurityAuth(config_.enableBonding, false, true);
  NimBLEDevice::setSecurityIOCap(BLE_HS_IO_NO_INPUT_OUTPUT);

  // 配置配对时交换哪些密钥。ENC 用于加密，ID 用于身份解析。
  // 这对后续“电脑记住设备并重连”更友好。
  NimBLEDevice::setSecurityInitKey(BLE_SM_PAIR_KEY_DIST_ENC |
                                   BLE_SM_PAIR_KEY_DIST_ID);
  NimBLEDevice::setSecurityRespKey(BLE_SM_PAIR_KEY_DIST_ENC |
                                   BLE_SM_PAIR_KEY_DIST_ID);

  // 创建 BLE Server。ESP32-S3 作为外设时，核心对象就是 server。
  server_ = NimBLEDevice::createServer();
  server_->setCallbacks(this);

  // 创建 Status Deck 专用服务。
  // 电脑端扫描到设备后，应优先检查这个 Service UUID。
  NimBLEService *service = server_->createService(config_.serviceUuid);

  // RX 特征值：电脑端 -> ESP32-S3。
  // WRITE 支持普通写入，WRITE_NR 支持不等待响应的快速写入。
  // 后续如果频繁刷新状态，可以让电脑端使用 Write Without Response。
  NimBLECharacteristic *rxCharacteristic = service->createCharacteristic(
      config_.rxCharacteristicUuid,
      NIMBLE_PROPERTY::WRITE | NIMBLE_PROPERTY::WRITE_NR);
  rxCharacteristic->setCallbacks(this);

  // TX 特征值：ESP32-S3 -> 电脑端。
  // READ 允许电脑端读取当前值，NOTIFY 允许 ESP32-S3 主动推送。
  txCharacteristic_ = service->createCharacteristic(
      config_.txCharacteristicUuid,
      NIMBLE_PROPERTY::READ | NIMBLE_PROPERTY::NOTIFY);

  // 初始值主要用于调试：电脑端连接后读取 TX，可以看到设备已准备好。
  // 不使用 setValue("...") 的泛型重载。部分 NimBLE 版本会把字符串指针本身
  // 当成 4 字节数据保存，而不是保存字符串内容。
  const char readyMessage[] = "ready";
  txCharacteristic_->setValue(
      reinterpret_cast<const uint8_t *>(readyMessage), sizeof(readyMessage) - 1);

  // 服务创建完以后必须 start，随后才能广播。
  service->start();
  startAdvertising();
}

void StatusBleServer::loop() {
  // 在 Arduino loop 任务中执行 JSON 解析与业务逻辑。必须在处理心跳和广播之前
  // 及时排空队列，否则连续 chunk 会填满这个有意保持很小的缓冲区。
  if (inboundQueue_ != nullptr && messageHandler_ != nullptr) {
    InboundMessage message;
    while (xQueueReceive(inboundQueue_, &message, 0) == pdTRUE) {
      messageHandler_(String(message.data));
    }
  }

  const uint32_t now = millis();

  // BLE 物理链路可能尚未触发断开回调，但桌面应用已经退出或电脑休眠。
  // 心跳超时后仅标记业务离线，不强制断开 BLE；这样桌面端恢复时可以立刻续传。
  if (connected_ && desktopOnline_ &&
      now - lastDesktopMessageMs_ > kDesktopHeartbeatTimeoutMs) {
    desktopOnline_ = false;
    Serial.println("Desktop heartbeat timed out");
  }

  // 已连接时不需要广播。
  // 未连接时也不要过于频繁检查，1 秒一次足够。
  if (connected_ || now - lastAdvertiseCheckMs_ < kAdvertiseCheckIntervalMs) {
    return;
  }

  lastAdvertiseCheckMs_ = now;

  // 如果因为断线或 BLE 栈状态变化导致广播停止，这里会补一次。
  if (!advertising_) {
    startAdvertising();
  }
}

bool StatusBleServer::isConnected() const { return connected_; }

bool StatusBleServer::isDesktopOnline() const {
  return connected_ && desktopOnline_;
}

bool StatusBleServer::hasBondedPeer() const {
  return NimBLEDevice::getNumBonds() > 0;
}

void StatusBleServer::setMessageHandler(MessageHandler handler) {
  messageHandler_ = handler;
}

bool StatusBleServer::notify(const String &message) {
  // notify 只能发给已经连接并订阅通知的中心设备。
  // 如果电脑端还没连接，或者 TX 特征值还没创建，直接返回 false。
  if (!connected_ || txCharacteristic_ == nullptr) {
    return false;
  }

  // BLE 单包数据长度有限。后续如果要发较大的 JSON，需要在协议层做分片。
  // 当前 MVP 先假设消息较短，用于 ACK、连接状态和调试信息。
  // 串口日志用于与桌面端的 notify 日志交叉验证：若这里完整打印 JSON、
  // 但桌面端仍只收到异常字节，则问题位于 macOS BLE 客户端实现而非 ESP32。
  Serial.print("BLE TX bytes: ");
  Serial.println(message.length());
  Serial.print("BLE TX: ");
  Serial.println(message);

  // 必须同时传入字节指针与长度。只传 message.c_str() 会命中 NimBLE 的
  // 泛型模板重载，并把 char* 地址误当作消息内容发送，表现为固定的 4 字节乱码。
  txCharacteristic_->setValue(
      reinterpret_cast<const uint8_t *>(message.c_str()), message.length());
  txCharacteristic_->notify();
  return true;
}

void StatusBleServer::startAdvertising() {
  NimBLEAdvertising *advertising = NimBLEDevice::getAdvertising();

  // 重新配置广播内容，避免断线重启广播时残留旧配置。
  advertising->reset();

  // 把 Status Deck 的 Service UUID 放进广播包。
  // 桌面客户端可以只扫描包含这个 UUID 的设备。
  advertising->addServiceUUID(config_.serviceUuid);
  advertising->setScanResponse(true);

  // BLE Appearance。0x0080 是 Generic Computer，主要用于调试时更容易识别。
  // 后续也可以改成更贴近自定义设备的值；不影响通信协议。
  advertising->setAppearance(0x0080);

  // 设备名放在 scan response 里，避免广播包空间不够。
  NimBLEAdvertisementData scanResponse;
  scanResponse.setName(config_.deviceName);
  advertising->setScanResponseData(scanResponse);

  // 开始广播。此后电脑端就可以扫描到并主动连接。
  NimBLEDevice::startAdvertising();
  advertising_ = true;

  Serial.print("BLE advertising as ");
  Serial.println(config_.deviceName);
}

void StatusBleServer::onConnect(NimBLEServer *server) {
  // 有电脑端连接后，停止认为自己在广播。
  // BLE 外设通常同一时间只服务一个中心设备，这对桌面状态卡足够。
  connected_ = true;
  advertising_ = false;
  // 刚连上时还不能算“桌面应用在线”，要等 hello 或 heartbeat 到达。
  desktopOnline_ = false;
  lastDesktopMessageMs_ = 0;
  Serial.println("BLE connected");
}

void StatusBleServer::onDisconnect(NimBLEServer *server) {
  // 电脑端断开后，立刻恢复广播。
  // 这样电脑端客户端重启后可以再次发现设备。
  connected_ = false;
  advertising_ = false;
  desktopOnline_ = false;
  lastDesktopMessageMs_ = 0;
  Serial.println("BLE disconnected");
  startAdvertising();
}

void StatusBleServer::onWrite(NimBLECharacteristic *characteristic) {
  // 上层没有注册回调时，收到的数据直接忽略。
  // 这样 BLE 层可以独立工作，不强制依赖 UI 或协议解析代码。
  if (messageHandler_ == nullptr) {
    return;
  }

  // NimBLE 返回 std::string。回调本身运行在 nimble_host 任务，不能在这里进行
  // ArduinoJson 解析、字符串拼接或显示刷新，否则很容易耗尽蓝牙任务栈。
  std::string value = characteristic->getValue();
  Serial.print("BLE write bytes: ");
  Serial.println(value.length());

  if (value.length() >= kInboundMessageBytes) {
    Serial.println("BLE write rejected: message exceeds inbound queue limit");
    return;
  }
  if (inboundQueue_ == nullptr) {
    Serial.println("BLE write dropped: inbound queue unavailable");
    return;
  }

  InboundMessage message;
  memcpy(message.data, value.data(), value.length());
  message.data[value.length()] = '\0';
  if (xQueueSend(inboundQueue_, &message, 0) != pdTRUE) {
    // 队列满说明桌面端写入速度超过当前 loop 的处理速度。分片发送使用带响应写入，
    // 正常情况下不会发生；丢弃后由下一轮完整状态同步恢复即可。
    Serial.println("BLE write dropped: inbound queue full");
    return;
  }

  // 已成功进入业务队列的消息视为桌面端在线。hello、heartbeat 和各个 xxx.update
  // 都会刷新这个时间；真正的 JSON 解析稍后由 loop 中的回调完成。
  lastDesktopMessageMs_ = millis();
  desktopOnline_ = true;
}
