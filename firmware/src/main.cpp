#include <Arduino.h>
#include <ArduinoJson.h>
#include <cstring>

#include "CodexUsageStore.h"
#include "DeviceStatusStore.h"
#include "StatusBleServer.h"

// 全局 BLE 服务对象。
// 后续屏幕 UI、状态缓存、协议解析都可以围绕这个对象收发数据。
StatusBleServer statusBle;

// 设备端最新状态。未来屏幕 UI 直接读取 deviceStatus.current() 渲染，
// 不需要知道 BLE 消息格式或重复解析 JSON。
DeviceStatusStore deviceStatus;

// Codex 用量有自己的存储器。未来 Codex 页面从 codexUsage.current() 读取，
// 因此不会和 CPU、内存等系统信息产生耦合。
CodexUsageStore codexUsage;

/**
 * 电脑端写入 BLE RX 特征值时会触发这个回调。
 *
 * 当前支持 hello、heartbeat 和 status.update：
 * - hello / heartbeat：确认通信链路仍正常
 * - status.update：解析系统信息与 Codex 用量，分别写入各自的状态存储器
 */
void handleBleMessage(const String &message) {
  Serial.print("BLE RX: ");
  Serial.println(message);

  // 文档规定的首版消息体不大。1024 bytes 足够容纳内存、磁盘和电源快照，
  // 后续引入更大页面数据时，应按 docs/ble-protocol.md 的 chunk 机制分片。
  StaticJsonDocument<1024> document;
  DeserializationError jsonError = deserializeJson(document, message);
  if (jsonError) {
    Serial.print("BLE JSON parse failed: ");
    Serial.println(jsonError.c_str());
    statusBle.notify("{\"v\":1,\"type\":\"error\",\"source\":\"device\",\"payload\":{\"code\":\"invalid_json\"}}");
    return;
  }

  const char *type = document["type"] | "";
  if (strcmp(type, "status.update") == 0) {
    String statusError;
    if (!deviceStatus.updateFromStatusPayload(document["payload"], statusError)) {
      Serial.print("status.update rejected: ");
      Serial.println(statusError);
      statusBle.notify("{\"v\":1,\"type\":\"error\",\"source\":\"device\",\"payload\":{\"code\":\"invalid_status\"}}");
      return;
    }

    if (!codexUsage.updateFromStatusPayload(document["payload"], statusError)) {
      Serial.print("Codex usage rejected: ");
      Serial.println(statusError);
      statusBle.notify("{\"v\":1,\"type\":\"error\",\"source\":\"device\",\"payload\":{\"code\":\"invalid_codex\"}}");
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
  }

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
}
