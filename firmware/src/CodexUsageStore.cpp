#include "CodexUsageStore.h"

#include <cstring>

bool CodexUsageStore::updateFromStatusPayload(JsonVariantConst payload,
                                              String &error) {
  // 缺少 codex 是旧桌面端的合法行为，不应导致整条 status.update 被拒绝。
  if (!payload.containsKey("codex")) {
    return true;
  }

  JsonObjectConst codex = payload["codex"].as<JsonObjectConst>();
  if (codex.isNull() || !codex["available"].is<bool>()) {
    error = "missing or invalid payload.codex.available";
    return false;
  }

  CodexUsageStatus next = status_;
  next.available = codex["available"].as<bool>();
  next.updatedAtMs = millis();

  // 没有真实来源时清掉旧额度，防止 UI 把过时信息误显示为当前可用额度。
  if (!next.available) {
    next.fiveHour = CodexUsageWindowStatus{};
    next.weekly = CodexUsageWindowStatus{};
    status_ = next;
    return true;
  }

  if (!parseWindow(codex["fiveHour"].as<JsonObjectConst>(), next.fiveHour,
                   error) ||
      !parseWindow(codex["weekly"].as<JsonObjectConst>(), next.weekly,
                   error)) {
    return false;
  }

  status_ = next;
  return true;
}

const CodexUsageStatus &CodexUsageStore::current() const { return status_; }

bool CodexUsageStore::parseWindow(JsonObjectConst object,
                                  CodexUsageWindowStatus &target,
                                  String &error) {
  if (object.isNull()) {
    error = "missing Codex usage window";
    return false;
  }
  if (!object["remainingPercent"].is<float>() &&
      !object["remainingPercent"].is<double>() &&
      !object["remainingPercent"].is<int>()) {
    error = "missing or invalid Codex remainingPercent";
    return false;
  }

  const char *resetLabel = object["resetLabel"].as<const char *>();
  if (resetLabel == nullptr) {
    error = "missing or invalid Codex resetLabel";
    return false;
  }

  target.valid = true;
  target.remainingPercent = object["remainingPercent"].as<float>();
  copyText(target.resetLabel, sizeof(target.resetLabel), resetLabel);
  return true;
}

void CodexUsageStore::copyText(char *target, size_t targetSize,
                               const char *value) {
  if (targetSize == 0) {
    return;
  }
  strncpy(target, value, targetSize - 1);
  target[targetSize - 1] = '\0';
}
