#pragma once

#include <Arduino.h>

/**
 * ActivityIndicator
 *
 * 管理“收到桌面端数据”的短促闪烁反馈。它使用 millis() 计时，不调用 delay()，
 * 因此闪灯期间 BLE 接收、显示刷新和断线广播仍可继续运行。
 *
 * pin 传入负数时会自动禁用实体 LED。这适用于 LED 与其他外设引脚冲突的测试
 * 阶段；调用 pulse() 仍然安全，只是不会操作 GPIO。
 */
class ActivityIndicator {
public:
  explicit ActivityIndicator(int pin, bool activeHigh = true);

  /** 配置 GPIO 并关闭指示灯。负数引脚会保持禁用。 */
  void begin();

  /** 从当前时刻开始闪亮一次；连续收到数据会延长本次亮灯时间。 */
  void pulse();

  /** 在 Arduino loop() 中持续调用，用于在超时后自动熄灯。 */
  void loop();

private:
  static constexpr uint32_t kPulseDurationMs = 55;

  void write(bool on);

  int pin_;
  bool activeHigh_;
  bool enabled_ = false;
  bool lit_ = false;
  uint32_t offAtMs_ = 0;
};
