#include "support/ActivityIndicator.h"

ActivityIndicator::ActivityIndicator(int pin, bool activeHigh)
    : pin_(pin), activeHigh_(activeHigh) {}

void ActivityIndicator::begin() {
  if (pin_ < 0) {
    return;
  }

  pinMode(pin_, OUTPUT);
  enabled_ = true;
  write(false);
}

void ActivityIndicator::pulse() {
  if (!enabled_) {
    return;
  }

  write(true);
  offAtMs_ = millis() + kPulseDurationMs;
}

void ActivityIndicator::loop() {
  if (!enabled_ || !lit_) {
    return;
  }

  // 使用有符号差值比较，可正确处理 millis() 大约 49 天后的溢出回绕。
  if (static_cast<int32_t>(millis() - offAtMs_) >= 0) {
    write(false);
  }
}

void ActivityIndicator::write(bool on) {
  lit_ = on;
  const bool level = activeHigh_ ? on : !on;
  digitalWrite(pin_, level ? HIGH : LOW);
}
