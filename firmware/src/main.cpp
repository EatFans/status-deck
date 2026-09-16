#include <Arduino.h>
#include "StatusBleServer.h"

// 全局 BLE 服务对象。
// 后续屏幕 UI、状态缓存、协议解析都可以围绕这个对象收发数据。
StatusBleServer statusBle;

/**
 * 电脑端写入 BLE RX 特征值时会触发这个回调。
 *
 * 当前先做最简单的调试处理：
 * - 把收到的消息打印到串口
 * - 通过 TX 特征值回一个 {"ok":true}
 *
 * 后续可以在这里解析 JSON，并把数据写入屏幕 UI 状态，例如：
 * - AI 用量
 * - API 余额
 * - 服务器在线状态
 * - 本机 CPU / 内存 / 电量
 */
void handleBleMessage(const String &message) {
  Serial.print("BLE RX: ");
  Serial.println(message);

  // 给电脑端一个最小 ACK，方便桌面客户端确认 ESP32-S3 已收到数据。
  statusBle.notify("{\"ok\":true}");
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
