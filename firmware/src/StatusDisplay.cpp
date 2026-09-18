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
                             int backlightPin, uint32_t spiFrequency)
    : csPin_(csPin), dcPin_(dcPin), resetPin_(resetPin),
      backlightPin_(backlightPin), spiFrequency_(spiFrequency),
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
  // Adafruit 驱动默认是 32MHz。ST7789 与经典 ESP32 的短线 SPI 连接可稳定跑在
  // 40MHz，能缩短卡片和进度条的绘制时间；如出现花屏，可在 platformio.ini
  // 将 STATUS_DECK_DISPLAY_SPI_FREQUENCY 改回 32000000。
  display_.setSPISpeed(spiFrequency_);
  display_.setRotation(0);
  display_.fillScreen(kBackground);
  display_.setTextWrap(false);
  available_ = true;
  Serial.printf("ST7789 ready: CS=%u DC=%u RESET=%u BL=%d SPI=%luHz\n",
                csPin_, dcPin_, resetPin_, backlightPin_,
                static_cast<unsigned long>(spiFrequency_));
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
  // 顶部采用手机式紧凑状态栏：不再显示大标题，把空间留给实时指标。
  const uint16_t iconColor = connected ? kBlue : kTextMuted;
  drawBluetoothIcon(214, 6, iconColor, 16);
  display_.fillRect(0, 29, kWidth, 1, kSurface);

  display_.setTextSize(1);
  display_.setTextColor(connected && desktopOnline ? kGreen : kTextMuted);
  display_.setCursor(12, 10);
  display_.print(connected ? (desktopOnline ? F("LIVE") : F("CONNECTED / IDLE"))
                           : F("WAITING FOR BLE"));
}

void StatusDisplay::drawSystemPage(const SystemStatus &system) {
  if (!system.valid) {
    // 蓝牙连接后直接进入系统页。初始同步尚未完成时，仅在页面内容区提示同步，
    // 不再显示“连接成功”的中间状态页。
    display_.setTextSize(1);
    display_.setTextColor(kTextPrimary);
    display_.setCursor(16, 46);
    display_.print(F("SYSTEM"));
    drawCenteredText(F("SYNCING METRICS"), 144, 1, kTextMuted);
    return;
  }

  display_.setTextSize(2);
  display_.setTextColor(kTextPrimary);
  display_.setCursor(16, 46);
  display_.print(F("SYSTEM"));

  // 系统页采用纵向进度条列表，保证 CPU、GPU、内存、磁盘、电源五项同时可见。
  drawMetricRow(F("CPU"), String(system.cpu.usedPercent, 0) + "%",
                system.cpu.usedPercent, true, 70, kBlue);
  drawMetricRow(F("GPU"),
                system.gpu.available ? String(system.gpu.usedPercent, 0) + "%"
                                     : String("--"),
                system.gpu.usedPercent, system.gpu.available, 108, kGreen);
  drawMetricRow(F("MEMORY"), String(system.memory.usedPercent, 0) + "%",
                system.memory.usedPercent, true, 146, kGreen);
  drawMetricRow(F("DISK"), String(system.disk.usedPercent, 0) + "%",
                system.disk.usedPercent, true, 184, kOrange);
  const float powerPercent = system.power.available && system.power.hasPercent
                                 ? static_cast<float>(system.power.percent)
                                 : 0.0f;
  String powerValue = "--";
  if (system.power.available && system.power.hasPercent) {
    powerValue = String(system.power.percent) + "%";
    if (system.power.hasCharging && system.power.charging) {
      powerValue += " CHG";
    }
  } else if (system.power.available) {
    powerValue = "AC";
  }
  drawMetricRow(F("POWER"), powerValue, powerPercent,
                system.power.available && system.power.hasPercent, 222,
                system.power.hasCharging && system.power.charging ? kGreen
                                                                   : kTextMuted);
}

void StatusDisplay::drawCodexPage(const CodexUsageStatus &codex) {
  display_.setTextSize(2);
  display_.setTextColor(kTextPrimary);
  display_.setCursor(16, 46);
  display_.print(F("CODEX"));

  if (!codex.available) {
    drawCenteredText(F("NO LOCAL USAGE DATA"), 130, 1, kTextMuted);
    drawCenteredText(F("USE CODEX ONCE FIRST"), 150, 1, kTextMuted);
    return;
  }

  // 额度卡片本身会覆盖旧数值，因此数据同步时不需要整页清屏。
  drawCodexWindow(F("5 HOUR"), codex.fiveHour.remainingPercent,
                  codex.fiveHour.resetLabel, 82, kBlue);
  drawCodexWindow(F("WEEKLY"), codex.weekly.remainingPercent,
                  codex.weekly.resetLabel, 184, kGreen);
}

void StatusDisplay::drawCodexWindow(const __FlashStringHelper *label,
                                    float remainingPercent,
                                    const String &resetLabel, int16_t y,
                                    uint16_t color) {
  constexpr int16_t kCardX = 16;
  constexpr int16_t kCardWidth = 208;
  constexpr int16_t kCardHeight = 86;
  display_.fillRoundRect(kCardX, y, kCardWidth, kCardHeight, 8, kSurface);

  display_.setTextSize(1);
  display_.setTextColor(kTextMuted);
  display_.setCursor(kCardX + 12, y + 12);
  display_.print(label);

  const String reset = formatResetLabel(resetLabel);
  const int16_t resetWidth = static_cast<int16_t>(reset.length()) * 6;
  display_.setCursor(kCardX + kCardWidth - 12 - resetWidth, y + 12);
  display_.print(reset);

  display_.setTextSize(4);
  display_.setTextColor(color);
  display_.setCursor(kCardX + 12, y + 30);
  display_.print(remainingPercent, 0);
  display_.setTextSize(2);
  display_.print('%');

  display_.setTextSize(1);
  display_.setTextColor(kTextMuted);
  display_.setCursor(kCardX + 112, y + 45);
  display_.print(F("REMAINING"));
  drawUsageBar(kCardX + 12, y + 68, kCardWidth - 24, remainingPercent,
               color);
}

void StatusDisplay::drawInitializationPage() {
  drawBluetoothIcon(104, 96, kBlue, 32);
  drawCenteredText(F("INITIALIZING"), 148, 2, kTextPrimary);
  drawCenteredText(F("WAITING FOR DESKTOP AGENT"), 180, 1, kTextMuted);
}

void StatusDisplay::drawBluetoothIcon(int16_t x, int16_t y, uint16_t color,
                                      uint8_t height) {
  // 蓝牙标识用线条绘制，不依赖外部位图资源。
  const int16_t centerX = x + height / 4;
  const int16_t rightX = x + height * 5 / 8;
  const int16_t leftX = x + height / 16;
  const int16_t upperY = y + height * 2 / 7;
  const int16_t lowerY = y + height * 5 / 7;
  display_.drawLine(centerX, y, centerX, y + height, color);
  display_.drawLine(centerX, y, rightX, upperY, color);
  display_.drawLine(rightX, upperY, leftX, lowerY, color);
  display_.drawLine(leftX, upperY, rightX, lowerY, color);
  display_.drawLine(centerX, y + height, rightX, lowerY, color);
}

void StatusDisplay::drawMetricRow(const __FlashStringHelper *label,
                                  const String &value, float percent,
                                  bool available, int16_t y,
                                  uint16_t color) {
  // 单行只有约 30 像素高，可在 240 x 320 屏幕中容纳五项核心开发指标。
  display_.fillRect(0, y, kWidth, 32, kBackground);
  display_.setTextSize(1);
  display_.setTextColor(kTextMuted);
  display_.setCursor(16, y);
  display_.print(label);
  const int16_t valueWidth = static_cast<int16_t>(value.length()) * 6;
  display_.setTextColor(available ? color : kTextMuted);
  display_.setCursor(224 - valueWidth, y);
  display_.print(value);
  if (available) {
    drawUsageBar(16, y + 15, 208, percent, color);
  } else {
    display_.drawRoundRect(16, y + 15, 208, 7, 3, kSurface);
  }
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

String StatusDisplay::formatResetLabel(const String &rawLabel) const {
  // 桌面端跨日期时会传入类似 "9月23日"。Adafruit 默认字库没有中文字形，
  // 因此这里只提取数字并格式化成 09/23；当天的 HH:MM 则直接显示。
  if (rawLabel.indexOf(':') >= 0) {
    return String(F("TODAY ")) + rawLabel;
  }

  int values[2] = {0, 0};
  uint8_t valueCount = 0;
  int current = -1;
  for (size_t index = 0; index < rawLabel.length(); ++index) {
    const char character = rawLabel[index];
    if (character >= '0' && character <= '9') {
      current = current < 0 ? character - '0' : current * 10 + character - '0';
      continue;
    }
    if (current >= 0 && valueCount < 2) {
      values[valueCount++] = current;
      current = -1;
    }
  }
  if (current >= 0 && valueCount < 2) {
    values[valueCount++] = current;
  }
  if (valueCount == 2) {
    char formatted[16];
    snprintf(formatted, sizeof(formatted), "RESET %02d/%02d", values[0],
             values[1]);
    return String(formatted);
  }
  return F("RESET --");
}
