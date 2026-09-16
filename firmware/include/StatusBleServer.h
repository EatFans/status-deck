#pragma once

#include <Arduino.h>
#include <NimBLEDevice.h>

/**
 * StatusBleServer
 *
 * Status Deck 的 BLE 通信封装。
 *
 * 当前 ESP32-S3 在 BLE 里扮演「外设 Peripheral」角色：
 * - ESP32-S3 负责广播自己，名字默认是 "Status Deck"
 * - 电脑端桌面客户端扮演「中心设备 Central」角色
 * - 电脑端扫描到设备后主动连接
 * - 电脑端通过 RX 特征值把 JSON/文本数据写给 ESP32-S3
 * - ESP32-S3 通过 TX 特征值 notify 回传状态或确认消息
 *
 * 关于“像键盘鼠标一样连接过一次后自动连接”：
 * BLE 外设通常不能主动连接电脑。更准确的机制是：
 * - ESP32-S3 持续使用固定设备名和固定 Service UUID 广播
 * - 开启 bonding 后，电脑端可以记住这个设备
 * - 下次 ESP32-S3 开机广播时，由电脑端自动或半自动重连
 *
 * 所以后续桌面客户端需要保存已连接过的设备标识，并在启动后自动扫描、
 * 匹配 Service UUID，然后主动重连。
 */
class StatusBleServer : private NimBLEServerCallbacks,
                        private NimBLECharacteristicCallbacks {
public:
  /**
   * 收到电脑端消息时的回调函数类型。
   *
   * 后续桌面客户端可以把状态数据序列化成 JSON，例如：
   * {"type":"usage","openai":{"used":12.5,"limit":100}}
   *
   * 固件这里先把收到的内容交给上层，后面可以在 main.cpp 或 UI 层解析。
   */
  using MessageHandler = void (*)(const String &message);

  /**
   * BLE 服务配置。
   *
   * deviceName:
   *   电脑蓝牙扫描列表里看到的名字。
   *
   * serviceUuid:
   *   Status Deck 专用 BLE 服务 UUID。电脑端可以靠它过滤设备，
   *   避免误连到耳机、键盘、鼠标或其他 BLE 设备。
   *
   * rxCharacteristicUuid:
   *   RX 是从 ESP32-S3 角度命名的 Receive。
   *   电脑端要把数据写入这个特征值。
   *
   * txCharacteristicUuid:
   *   TX 是从 ESP32-S3 角度命名的 Transmit。
   *   ESP32-S3 用 notify 从这个特征值向电脑端推送消息。
   *
   * enableBonding:
   *   开启后允许电脑端和 ESP32-S3 建立绑定关系。
   *   这有助于电脑端记住设备，实现后续更顺滑的自动重连体验。
   */
  struct Config {
    const char *deviceName = "Status Deck";
    const char *serviceUuid = "7f3a0001-6c21-4b7d-9d5d-1f4f2b3a9000";
    const char *rxCharacteristicUuid = "7f3a0002-6c21-4b7d-9d5d-1f4f2b3a9000";
    const char *txCharacteristicUuid = "7f3a0003-6c21-4b7d-9d5d-1f4f2b3a9000";
    bool enableBonding = true;
  };

  /**
   * 初始化 BLE 设备、服务、特征值，并开始广播。
   *
   * 这个方法通常只在 setup() 里调用一次。
   */
  void begin(const Config &config = Config());

  /**
   * BLE 维护循环。
   *
   * 当前主要用于断开连接后检查广播状态，保证设备继续可被电脑端发现。
   * 在 Arduino 的 loop() 里持续调用即可。
   */
  void loop();

  /**
   * 当前是否已有电脑端连接。
   */
  bool isConnected() const;

  /**
   * 是否已经存在 bonded peer。
   *
   * 注意：这只能说明 ESP32-S3 侧保存过绑定信息；
   * 真正的自动重连仍然需要电脑端主动扫描和连接。
   */
  bool hasBondedPeer() const;

  /**
   * 设置电脑端写入 RX 特征值后的消息处理函数。
   */
  void setMessageHandler(MessageHandler handler);

  /**
   * 通过 TX 特征值向已连接的电脑端发送 notify。
   *
   * 返回 false 通常表示当前还没有电脑端连接。
   */
  bool notify(const String &message);

private:
  /**
   * 启动广播。
   *
   * 广播里会带上 serviceUuid，方便桌面客户端只扫描 Status Deck 设备。
   */
  void startAdvertising();

  /**
   * NimBLE 回调：电脑端连接成功。
   */
  void onConnect(NimBLEServer *server) override;

  /**
   * NimBLE 回调：电脑端断开连接。
   *
   * 断开后会重新开始广播，让电脑端可以再次连接。
   */
  void onDisconnect(NimBLEServer *server) override;

  /**
   * NimBLE 回调：电脑端向 RX 特征值写入数据。
   */
  void onWrite(NimBLECharacteristic *characteristic) override;

  // 当前 BLE 配置。保存下来是为了断线后重新广播时继续使用同一组 UUID。
  Config config_;

  // NimBLE server 对象由 NimBLE 库创建和持有，这里只保存指针方便回调管理。
  NimBLEServer *server_ = nullptr;

  // TX 特征值。ESP32-S3 通过它 notify 数据给电脑端。
  NimBLECharacteristic *txCharacteristic_ = nullptr;

  // 用户层消息回调。收到电脑端写入的数据后会调用它。
  MessageHandler messageHandler_ = nullptr;

  // 当前连接状态。
  bool connected_ = false;

  // 当前是否认为自己正在广播。用于断线后的广播恢复。
  bool advertising_ = false;

  // 上一次检查广播状态的时间，避免 loop() 每次都重复启动广播。
  uint32_t lastAdvertiseCheckMs_ = 0;
};
