#pragma once

#include <Arduino.h>
#include <ArduinoJson.h>

/**
 * CodexUsageStore
 *
 * 专门保存 Codex 剩余额度的设备端存储器。
 *
 * 它刻意不放进 DeviceStatusStore：系统资源和 Codex 用量拥有不同的数据来源、
 * 更新频率及页面生命周期。显示屏未来需要渲染 Codex 页面时，只读取这里的
 * current()，不接触 JSON 或 BLE 回调。
 */

struct CodexUsageWindowStatus {
  bool valid = false;
  float remainingPercent = 0.0f;
  char resetLabel[32] = {};
};

struct CodexUsageStatus {
  // false 表示桌面端当前没有获取到可信的 Codex 用量，不等于额度已耗尽。
  bool available = false;
  uint32_t updatedAtMs = 0;
  CodexUsageWindowStatus fiveHour;
  CodexUsageWindowStatus weekly;
};

class CodexUsageStore {
public:
  /**
   * 从 status.update 的 payload 中读取 payload.codex。
   *
   * codex 字段不存在时返回 true 且保留现有内容，方便兼容旧版桌面端。若字段
   * 存在但格式错误则返回 false，旧数据同样不会被半截数据覆盖。
   */
  bool updateFromStatusPayload(JsonVariantConst payload, String &error);

  /** 返回当前完整 Codex 用量状态，供未来显示页面读取。 */
  const CodexUsageStatus &current() const;

private:
  bool parseWindow(JsonObjectConst object, CodexUsageWindowStatus &target,
                   String &error);
  void copyText(char *target, size_t targetSize, const char *value);

  CodexUsageStatus status_;
};
