#pragma once

#include <Adafruit_GFX.h>
#include <Adafruit_ST7789.h>
#include <Arduino.h>

#include "status/CodexUsageStore.h"
#include "status/DeviceStatusStore.h"

/**
 * StatusDisplay
 *
 * 2.8 英寸 ST7789 SPI 彩屏的状态卡 UI。
 *
 * 显示模块只读取两个状态存储器，不解析 JSON、不直接访问 BLE。这样以后换成
 * 彩色屏或触控屏时，可以整体替换本类，协议处理和状态缓存无需改动。
 */
class StatusDisplay {
public:
  StatusDisplay(uint8_t csPin, uint8_t dcPin, uint8_t resetPin,
                int backlightPin, uint32_t spiFrequency);

  /**
   * 初始化 VSPI 与 ST7789。屏幕固定使用 240 x 320 纵向分辨率；返回 false 时
   * 固件仍继续运行，只是不渲染屏幕，便于单独排查接线。
   */
  bool begin();

  /**
   * 依据最新状态按需刷新屏幕。
   *
   * 当前页由上位机通过上一页、下一页命令切换；数据接收后只更新当前页对应
   * 的区域，避免后台页面的数据造成可见闪烁。
   */
  void render(const SystemStatus &system, const CodexUsageStatus &codex,
              bool connected, bool desktopOnline);

  /** 标记有新状态，下一次 render() 会立刻重绘。 */
  void markDirty();

  /** 标记系统指标已更新；仅在系统页可见时刷新屏幕。 */
  void markSystemDirty();

  /** 标记 Codex 额度已更新；仅在 Codex 页可见时刷新屏幕。 */
  void markCodexDirty();

  /** 切换到上一页或下一页；返回 true 表示当前页面已经改变。 */
  bool showPreviousPage();
  bool showNextPage();

  /** 当前显示页由设备保存，供 BLE 协议同步到桌面端。 */
  uint8_t currentPageIndex() const;
  const char *currentPageId() const;
  uint8_t pageCount() const;

  /** 屏幕可用时返回 true，供启动日志和后续设置页面使用。 */
  bool available() const;

private:
  static constexpr uint16_t kWidth = 240;
  static constexpr uint16_t kHeight = 320;
  static constexpr uint8_t kSystemPageIndex = 1;
  static constexpr uint8_t kCodexPageIndex = 2;

  void drawHeader(bool connected, bool desktopOnline);
  void drawBluetoothIcon(int16_t x, int16_t y, uint16_t color,
                         uint8_t height);
  void drawSystemPage(const SystemStatus &system);
  void drawCodexPage(const CodexUsageStatus &codex);
  void drawCodexWindow(const __FlashStringHelper *label, float remainingPercent,
                       const String &resetLabel, int16_t y, uint16_t color);
  void drawInitializationPage();
  void drawMetricRow(const __FlashStringHelper *label, const String &value,
                     float percent, bool available, int16_t y,
                     uint16_t color);
  void drawUsageBar(int16_t x, int16_t y, int16_t width, float percent,
                    uint16_t color);
  void drawCenteredText(const String &text, int16_t y, uint8_t size,
                        uint16_t color);
  String formatResetLabel(const String &rawLabel) const;
  Adafruit_GFX &graphics();
  void flushFrameBuffer();
  void flushFrameBufferRect(int16_t x, int16_t y, int16_t width,
                            int16_t height);
  void flushSystemMetrics();
  void flushCodexWindows();
  bool isSystemPage() const;

  uint8_t csPin_;
  uint8_t dcPin_;
  uint8_t resetPin_;
  int backlightPin_;
  uint32_t spiFrequency_;
  Adafruit_ST7789 display_;
  GFXcanvas16 *frameBuffer_ = nullptr;
  bool available_ = false;
  bool doubleBuffered_ = false;
  bool dirty_ = true;
  bool systemDirty_ = true;
  bool codexDirty_ = true;
  uint8_t currentPageIndex_ = kSystemPageIndex;
  // 首帧、页面切换或连接状态变化时才需要清空整个屏幕；普通数据同步只更新
  // 数值区域，避免 SPI 彩屏在两次同步之间出现肉眼可见的黑屏闪烁。
  bool frameKnown_ = false;
  // BLE 连接变化不是业务数据更新，也必须触发重绘，避免断线后残留旧监控数据。
  bool connectionKnown_ = false;
  bool lastConnected_ = false;
  bool lastDesktopOnline_ = false;
  bool lastSystemValid_ = false;
  bool lastCodexAvailable_ = false;
};
