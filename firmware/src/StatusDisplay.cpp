#include "StatusDisplay.h"

#include <SPI.h>

namespace {
constexpr uint16_t kBackground = ST77XX_BLACK;
constexpr uint16_t kSurface = 0x18E3;
constexpr uint16_t kTextPrimary = ST77XX_WHITE;
constexpr uint16_t kTextMuted = 0x9CD3;
constexpr uint16_t kBlue = 0x55FF;
constexpr uint16_t kGreen = 0x3ECA;
constexpr uint16_t kOrange = 0xFD20;
constexpr uint16_t kRed = 0xF9E7;
} // namespace

StatusDisplay::StatusDisplay(uint8_t csPin, uint8_t dcPin, uint8_t resetPin,
                             int backlightPin)
    : csPin_(csPin), dcPin_(dcPin), resetPin_(resetPin),
      backlightPin_(backlightPin),
      display_(csPin, dcPin, resetPin) {}

bool StatusDisplay::begin() {
  // VSPI 使用经典 ESP32 的硬件默认引脚：SCK=18、MISO=19、MOSI=23。
  // 当前屏幕不读取 MISO，仍传入它以保留硬件 SPI 总线的标准配置。
  SPI.begin(18, 19, 23, csPin_);
  // 这块转接板的 LED 是背光控制输入，而不是背光电源正极。GPIO25 输出高电平
  // 即可打开板载三极管，不需要与 VCC 共用 ESP32 唯一的 3V3 针脚。
  if (backlightPin_ >= 0) {
    pinMode(backlightPin_, OUTPUT);
    digitalWrite(backlightPin_, HIGH);
  }
  display_.init(240, 320);
  display_.setRotation(0);
  display_.fillScreen(kBackground);
  display_.setTextWrap(false);
  available_ = true;
  Serial.printf(
      "ST7789 ready: CS=%u DC=%u RESET=%u BL=%d, SPI SCK=18 MOSI=23\n",
      csPin_, dcPin_, resetPin_, backlightPin_);
  return true;
}

void StatusDisplay::render(const SystemStatus &system,
                           const CodexUsageStatus &codex, bool connected,
                           bool desktopOnline) {
  if (!available_) {
    return;
  }

  const uint32_t now = millis();
  // 断线本身不会修改 SystemStatus 或 CodexUsageStatus，因此在这里单独观察连接
  // 状态。变化后强制重绘，让屏幕立即回到初始化页面，不展示已过期的旧数据。
  if (!connectionKnown_ || connected != lastConnected_) {
    connectionKnown_ = true;
    lastConnected_ = connected;
    dirty_ = true;
  }

  // 未连接时固定停留在初始化页，避免每 4 秒无意义地清屏一次。
  const bool hasContent = system.valid || codex.available;
  bool pageChanged = false;
  if (connected && hasContent &&
      static_cast<int32_t>(now - pageStartedAtMs_) >=
          static_cast<int32_t>(kPageDurationMs)) {
    systemPage_ = !systemPage_;
    pageStartedAtMs_ = now;
    pageChanged = true;
  }

  // 当数据从“等待”切换为“可显示”或反向变化时，需要清掉旧页面；同理，桌面端
  // 在线状态变化也需要刷新顶部状态文字。除此以外，不进行整屏清除。
  const bool layoutChanged =
      !frameKnown_ || pageChanged || lastDesktopOnline_ != desktopOnline ||
      lastSystemValid_ != system.valid || lastCodexAvailable_ != codex.available;
  if (!dirty_ && !layoutChanged) {
    return;
  }

  if (layoutChanged) {
    display_.fillScreen(kBackground);
    drawHeader(connected, desktopOnline);
    if (!connected) {
      drawInitializationPage();
    } else if (!hasContent) {
      drawWaitingPage();
    } else if (systemPage_) {
      drawSystemPage(system);
    } else {
      drawCodexPage(codex);
    }
  } else if (systemPage_) {
    // 系统页的四张卡片本身使用不透明背景覆盖旧值，因此只重画这些卡片即可。
    drawSystemPage(system);
  } else {
    // Codex 页仅擦除动态数值区域，保留标题、说明和顶部状态栏不动。
    drawCodexPage(codex);
  }

  frameKnown_ = true;
  lastDesktopOnline_ = desktopOnline;
  lastSystemValid_ = system.valid;
  lastCodexAvailable_ = codex.available;
  dirty_ = false;
}

void StatusDisplay::markDirty() {
  dirty_ = true;
}

bool StatusDisplay::available() const { return available_; }

void StatusDisplay::drawHeader(bool connected, bool desktopOnline) {
  display_.setTextSize(2);
  display_.setTextColor(kTextPrimary);
  display_.setCursor(16, 18);
  display_.print(F("STATUS DECK"));

  const uint16_t iconColor = connected ? kBlue : kTextMuted;
  drawBluetoothIcon(204, 16, iconColor);
  display_.fillRect(0, 52, kWidth, 2, kSurface);

  display_.setTextSize(1);
  display_.setTextColor(connected && desktopOnline ? kGreen : kTextMuted);
  display_.setCursor(16, 62);
  display_.print(connected ? (desktopOnline ? F("CONNECTED") : F("CONNECTED / IDLE"))
                           : F("WAITING FOR BLE"));
}

void StatusDisplay::drawSystemPage(const SystemStatus &system) {
  if (!system.valid) {
    drawWaitingPage();
    return;
  }

  display_.setTextSize(2);
  display_.setTextColor(kTextPrimary);
  display_.setCursor(16, 90);
  display_.print(F("SYSTEM"));

  drawMetricCard(F("CPU"), system.cpu.usedPercent, 16, 120, kBlue);
  drawMetricCard(F("MEMORY"), system.memory.usedPercent, 128, 120, kGreen);
  drawMetricCard(F("DISK"), system.disk.usedPercent, 16, 220, kOrange);

  const float powerPercent = system.power.available && system.power.hasPercent
                                 ? static_cast<float>(system.power.percent)
                                 : 0.0f;
  drawMetricCard(F("POWER"), powerPercent, 128, 220,
                 system.power.hasCharging && system.power.charging ? kGreen
                                                                    : kTextMuted);

  display_.setTextSize(1);
  display_.setTextColor(kTextMuted);
  // GPU 文本长度可能变化，例如 "GPU 100%" 变成 "GPU 9%"；先擦掉这一小块
  // 区域，避免末尾残留字符，而不是清空整块屏幕。
  display_.fillRect(0, 300, kWidth, 20, kBackground);
  display_.setCursor(16, 304);
  if (system.gpu.available) {
    display_.print(F("GPU  "));
    display_.print(system.gpu.usedPercent, 0);
    display_.print('%');
  } else {
    display_.print(F("LIVE DESKTOP METRICS"));
  }
}

void StatusDisplay::drawCodexPage(const CodexUsageStatus &codex) {
  display_.setTextSize(2);
  display_.setTextColor(kTextPrimary);
  display_.setCursor(16, 90);
  display_.print(F("CODEX USAGE"));

  if (!codex.available) {
    drawCenteredText(F("NO LOCAL USAGE DATA"), 160, 1, kTextMuted);
    drawCenteredText(F("USE CODEX ONCE FIRST"), 180, 1, kTextMuted);
    return;
  }

  // 只清除会随同步数据变化的两个区域。每次桌面端推送数据时不再产生全屏黑帧。
  display_.fillRect(12, 144, 216, 82, kBackground);
  display_.fillRect(12, 258, 216, 50, kBackground);

  display_.setTextSize(1);
  display_.setTextColor(kTextMuted);
  display_.setCursor(16, 130);
  display_.print(F("5 HOUR WINDOW"));
  display_.setTextSize(5);
  display_.setTextColor(kBlue);
  display_.setCursor(16, 148);
  display_.print(codex.fiveHour.remainingPercent, 0);
  display_.setTextSize(2);
  display_.print('%');
  drawUsageBar(16, 202, 208, codex.fiveHour.remainingPercent, kBlue);
  display_.setTextSize(1);
  display_.setTextColor(kTextMuted);
  display_.setCursor(16, 216);
  display_.print(F("RESET "));
  display_.print(codex.fiveHour.resetLabel);

  display_.setCursor(16, 246);
  display_.print(F("WEEKLY WINDOW"));
  display_.setTextSize(3);
  display_.setTextColor(kGreen);
  display_.setCursor(16, 262);
  display_.print(codex.weekly.remainingPercent, 0);
  display_.setTextSize(1);
  display_.print('%');
  drawUsageBar(120, 272, 104, codex.weekly.remainingPercent, kGreen);
  display_.setTextSize(1);
  display_.setTextColor(kTextMuted);
  display_.setCursor(16, 294);
  display_.print(F("RESET "));
  display_.print(codex.weekly.resetLabel);
}

void StatusDisplay::drawInitializationPage() {
  drawBluetoothIcon(104, 124, kBlue);
  drawCenteredText(F("INITIALIZING"), 176, 2, kTextPrimary);
  drawCenteredText(F("WAITING FOR DESKTOP AGENT"), 208, 1, kTextMuted);
  drawCenteredText(F("STATUS DECK"), 274, 1, kTextMuted);
}

void StatusDisplay::drawWaitingPage() {
  drawCenteredText(F("CONNECTED"), 166, 2, kTextPrimary);
  drawCenteredText(F("WAITING FOR INITIAL DATA"), 198, 1, kTextMuted);
}

void StatusDisplay::drawBluetoothIcon(int16_t x, int16_t y, uint16_t color) {
  // 蓝牙标识用线条绘制，不依赖外部位图资源。
  display_.drawLine(x + 8, y, x + 8, y + 28, color);
  display_.drawLine(x + 8, y, x + 18, y + 8, color);
  display_.drawLine(x + 18, y + 8, x + 2, y + 20, color);
  display_.drawLine(x + 2, y + 8, x + 18, y + 20, color);
  display_.drawLine(x + 8, y + 28, x + 18, y + 20, color);
}

void StatusDisplay::drawMetricCard(const __FlashStringHelper *label,
                                   float percent, int16_t x, int16_t y,
                                   uint16_t color) {
  constexpr int16_t kCardWidth = 96;
  constexpr int16_t kCardHeight = 82;
  display_.fillRoundRect(x, y, kCardWidth, kCardHeight, 8, kSurface);
  display_.setTextSize(1);
  display_.setTextColor(kTextMuted);
  display_.setCursor(x + 10, y + 10);
  display_.print(label);
  display_.setTextSize(3);
  display_.setTextColor(color);
  display_.setCursor(x + 10, y + 30);
  display_.print(percent, 0);
  display_.setTextSize(1);
  display_.print('%');
  drawUsageBar(x + 10, y + 66, 76, percent, color);
}

void StatusDisplay::drawUsageBar(int16_t x, int16_t y, int16_t width,
                                 float percent, uint16_t color) {
  constexpr int16_t kHeight = 7;
  const float clamped = constrain(percent, 0.0f, 100.0f);
  const int16_t fillWidth = static_cast<int16_t>((width - 2) * clamped / 100.0f);
  display_.fillRoundRect(x, y, width, kHeight, 3, kBackground);
  display_.drawRoundRect(x, y, width, kHeight, 3, kTextMuted);
  if (fillWidth > 0) {
    display_.fillRoundRect(x + 1, y + 1, fillWidth, kHeight - 2, 2, color);
  }
}

void StatusDisplay::drawCenteredText(const String &text, int16_t y,
                                     uint8_t size, uint16_t color) {
  display_.setTextSize(size);
  display_.setTextColor(color);
  const int16_t width = static_cast<int16_t>(text.length()) * 6 * size;
  display_.setCursor((kWidth - width) / 2, y);
  display_.print(text);
}
