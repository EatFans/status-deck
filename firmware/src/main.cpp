#include <Arduino.h>
#include <ArduinoJson.h>
#include <cstring>

#include "ActivityIndicator.h"
#include "BleChunkAssembler.h"
#include "CodexUsageStore.h"
#include "DeviceStatusStore.h"
#include "StatusDisplay.h"
#include "StatusBleServer.h"

#ifndef STATUS_DECK_DISPLAY_CS
#define STATUS_DECK_DISPLAY_CS 5
#endif

#ifndef STATUS_DECK_DISPLAY_DC
#define STATUS_DECK_DISPLAY_DC 27
#endif

#ifndef STATUS_DECK_DISPLAY_RESET
#define STATUS_DECK_DISPLAY_RESET 26
#endif

#ifndef STATUS_DECK_DISPLAY_BACKLIGHT
#define STATUS_DECK_DISPLAY_BACKLIGHT 25
#endif

#ifndef STATUS_DECK_DISPLAY_SPI_FREQUENCY
#define STATUS_DECK_DISPLAY_SPI_FREQUENCY 40000000
#endif

#ifndef STATUS_DECK_ACTIVITY_LED_PIN
#define STATUS_DECK_ACTIVITY_LED_PIN 2
#endif

// 全局 BLE 服务对象。
// 后续屏幕 UI、状态缓存、协议解析都可以围绕这个对象收发数据。
StatusBleServer statusBle;

// 设备端最新状态。未来屏幕 UI 直接读取 deviceStatus.current() 渲染，
// 不需要知道 BLE 消息格式或重复解析 JSON。
DeviceStatusStore deviceStatus;

// Codex 用量有自己的存储器。未来 Codex 页面从 codexUsage.current() 读取，
// 因此不会和 CPU、内存等系统信息产生耦合。
CodexUsageStore codexUsage;

// 大于单次 GATT 写入上限的桌面端消息会先在这里重组，再进入下面原有的 JSON
// 解析和状态存储流程。显示层不需要知道 BLE 分片的存在。
BleChunkAssembler chunkAssembler;

// ST7789 彩色显示器。它只读取状态缓存，渲染逻辑不会进入 BLE 回调。
StatusDisplay statusDisplay(STATUS_DECK_DISPLAY_CS, STATUS_DECK_DISPLAY_DC,
                            STATUS_DECK_DISPLAY_RESET,
                            STATUS_DECK_DISPLAY_BACKLIGHT,
                            STATUS_DECK_DISPLAY_SPI_FREQUENCY);

// 收到有效桌面端数据时短闪。若未来硬件改版未提供可用 LED，可将配置设为 -1 禁用。
ActivityIndicator activityIndicator(STATUS_DECK_ACTIVITY_LED_PIN);

/**
 * 电脑端写入 BLE RX 特征值时会触发这个回调。
 *
 * 当前支持 hello、heartbeat、system.update、codex.update 与页面控制命令：
 * - hello / heartbeat：确认通信链路仍正常
 * - system.update：解析系统信息并写入 deviceStatus
 * - codex.update：解析 Codex 用量并写入 codexUsage
 * - page.previous / page.next：切换当前显示页并回传 page.status
 */
void notifyPageStatus() {
  char message[192];
  snprintf(message, sizeof(message),
           "{\"v\":1,\"type\":\"page.status\",\"source\":\"device\","
           "\"target\":\"desktop\",\"payload\":{\"index\":%u,\"id\":\"%s\","
           "\"count\":%u}}",
           static_cast<unsigned int>(statusDisplay.currentPageIndex()),
           statusDisplay.currentPageId(),
           static_cast<unsigned int>(statusDisplay.pageCount()));
  statusBle.notify(message);
}

void handleBleMessage(const String &message) {
  Serial.print("BLE RX: ");
  Serial.println(message);

  // 独立模块消息通常远小于单次 GATT 上限。即使通过 chunk 重组，当前协议也将
  // 单模块限制在 1024 bytes；该对象现在运行在 Arduino loop 任务，不占用 NimBLE
  // 蓝牙回调线程的栈。
  StaticJsonDocument<1024> document;
  DeserializationError jsonError = deserializeJson(document, message);
  if (jsonError) {
    Serial.print("BLE JSON parse failed: ");
    Serial.println(jsonError.c_str());
    statusBle.notify("{\"v\":1,\"type\":\"error\",\"source\":\"device\",\"payload\":{\"code\":\"invalid_json\"}}");
    return;
  }

  const char *type = document["type"] | "";
  if (strcmp(type, "chunk") == 0) {
    String completeMessage;
    String chunkError;
    if (!chunkAssembler.append(document["payload"].as<JsonObjectConst>(),
                               completeMessage, chunkError)) {
      Serial.print("BLE chunk rejected: ");
      Serial.println(chunkError);
      statusBle.notify("{\"v\":1,\"type\":\"error\",\"source\":\"device\",\"payload\":{\"code\":\"invalid_chunk\"}}");
      return;
    }

    // 未收齐时不 ACK，避免每个业务更新产生多条无用 notify；最后一个 chunk 收齐
    // 后递归走原始完整消息，现有业务成功路径会发送一条 ACK。
    if (completeMessage.length() > 0) {
      handleBleMessage(completeMessage);
    }
    return;
  }

  if (strcmp(type, "hello") == 0) {
    // 页面由设备保存。每次连接建立后立即回传，桌面端据此显示当前页并只同步
    // 此页所需的数据。
    notifyPageStatus();
  }

  if (strcmp(type, "page.previous") == 0) {
    if (statusDisplay.showPreviousPage()) {
      notifyPageStatus();
    }
  }

  if (strcmp(type, "page.next") == 0) {
    if (statusDisplay.showNextPage()) {
      notifyPageStatus();
    }
  }

  if (strcmp(type, "system.update") == 0) {
    String statusError;
    if (!deviceStatus.updateFromSystemPayload(document["payload"], statusError)) {
      Serial.print("system.update rejected: ");
      Serial.println(statusError);
      statusBle.notify("{\"v\":1,\"type\":\"error\",\"source\":\"device\",\"payload\":{\"code\":\"invalid_status\"}}");
      return;
    }

    // 仅打印摘要，避免每 2 秒刷出完整 JSON。屏幕 UI 可通过 current() 读取详情。
    const SystemStatus &system = deviceStatus.current();
    Serial.print("System status stored: cpu=");
    Serial.print(system.cpu.usedPercent, 1);
    Serial.print("% gpu=");
    if (system.gpu.available) {
      Serial.print(system.gpu.usedPercent, 1);
      Serial.print("%");
    } else {
      Serial.print("unavailable");
    }
    Serial.print(" memory=");
    Serial.print(system.memory.usedPercent, 1);
    Serial.print("% disk=");
    Serial.print(system.disk.usedPercent, 1);
    Serial.println("%");
    statusDisplay.markSystemDirty();
  }

  if (strcmp(type, "codex.update") == 0) {
    String statusError;
    if (!codexUsage.updateFromCodexPayload(document["payload"], statusError)) {
      Serial.print("codex.update rejected: ");
      Serial.println(statusError);
      statusBle.notify("{\"v\":1,\"type\":\"error\",\"source\":\"device\",\"payload\":{\"code\":\"invalid_codex\"}}");
      return;
    }

    const CodexUsageStatus &codex = codexUsage.current();
    if (!codex.available) {
      Serial.println("Codex usage unavailable");
    } else {
      Serial.print("Codex usage stored: 5h=");
      Serial.print(codex.fiveHour.remainingPercent, 1);
      Serial.print("% reset=");
      Serial.print(codex.fiveHour.resetLabel);
      Serial.print(" weekly=");
      Serial.print(codex.weekly.remainingPercent, 1);
      Serial.print("% reset=");
      Serial.println(codex.weekly.resetLabel);
    }
    statusDisplay.markCodexDirty();
  }

  // 只有数据完整且 JSON 合法时才闪烁，避免把损坏包误判为有效更新。
  // hello、heartbeat 与业务更新都会提供一次轻量的链路活动反馈。
  activityIndicator.pulse();

  // 给电脑端 ACK。后续可以从 document["id"] 提取请求 ID，填入 payload.ref。
  statusBle.notify("{\"v\":1,\"type\":\"ack\",\"source\":\"device\",\"payload\":{\"ok\":true}}");
}

/**
 * 初始化
 */
void setup() {
  // 串口用于开发阶段调试。PlatformIO monitor_speed 也配置为 115200。
  Serial.begin(115200);

  // 给 USB 串口一点初始化时间，避免刚启动时丢第一段日志。
  delay(200);

  activityIndicator.begin();
  if (statusDisplay.begin()) {
    statusDisplay.markDirty();
  }

  // 注册 BLE 消息回调。
  // 电脑端写 RX 特征值后，会进入 handleBleMessage()。
  statusBle.setMessageHandler(handleBleMessage);

  // 启动 BLE 外设：
  // - 设备名：Status Deck
  // - 服务 UUID：Status Deck 专用 UUID
  // - RX：电脑端写入数据
  // - TX：ESP32-S3 notify 回传数据
  statusBle.begin();

  Serial.println("Status Deck BLE server started");
}

/**
 * 循环
 */
void loop() {
  // 维护 BLE 状态。当前主要用于断线后恢复广播。
  // 后续屏幕刷新、按键扫描、状态动画也会放在 loop() 或独立模块里。
  statusBle.loop();
  activityIndicator.loop();
  statusDisplay.render(deviceStatus.current(), codexUsage.current(),
                       statusBle.isConnected(), statusBle.isDesktopOnline());
}
