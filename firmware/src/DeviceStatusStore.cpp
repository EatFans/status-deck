#include "DeviceStatusStore.h"

#include <cstring>

bool DeviceStatusStore::updateFromSystemPayload(JsonVariantConst payload,
                                                 String &error) {
  JsonObjectConst system = payload.as<JsonObjectConst>();
  if (system.isNull()) {
    error = "invalid system.update payload";
    return false;
  }

  // 先解析到临时副本。不同节奏的数据域会发送局部 payload；任何出现的字段
  // 类型不对时直接返回，保留屏幕正在展示的旧状态。
  SystemStatus next = status_;
  bool hasUpdate = false;
  if (system.containsKey("cpu")) {
    hasUpdate = true;
    if (!parseCPU(system["cpu"].as<JsonObjectConst>(), next.cpu, error)) {
      return false;
    }
  }
  if (system.containsKey("gpu")) {
    hasUpdate = true;
    if (!parseGPU(system["gpu"].as<JsonObjectConst>(), next.gpu, error)) {
      return false;
    }
  }
  if (system.containsKey("memory")) {
    hasUpdate = true;
    if (!parseMemory(system["memory"].as<JsonObjectConst>(), next.memory,
                     error)) {
      return false;
    }
  }
  if (system.containsKey("disk")) {
    hasUpdate = true;
    if (!parseDisk(system["disk"].as<JsonObjectConst>(), next.disk, error)) {
      return false;
    }
  }
  if (system.containsKey("power")) {
    hasUpdate = true;
    if (!parsePower(system["power"].as<JsonObjectConst>(), next.power,
                    error)) {
      return false;
    }
  }
  if (!hasUpdate) {
    error = "system.update has no recognized fields";
    return false;
  }

  next.valid = true;
  next.updatedAtMs = millis();
  status_ = next;
  return true;
}

const SystemStatus &DeviceStatusStore::current() const { return status_; }

bool DeviceStatusStore::parseCPU(JsonObjectConst object, CPUStatus &target,
                                 String &error) {
  if (object.isNull()) {
    error = "missing system.cpu";
    return false;
  }
  return requireFloat(object, "usagePercent", target.usedPercent, error);
}

bool DeviceStatusStore::parseGPU(JsonObjectConst object, GPUStatus &target,
                                 String &error) {
  if (object.isNull() || !object["available"].is<bool>()) {
    error = "missing or invalid system.gpu.available";
    return false;
  }

  target.available = object["available"].as<bool>();
  if (!target.available) {
    target.usedPercent = 0.0f;
    return true;
  }
  return requireFloat(object, "usagePercent", target.usedPercent, error);
}

bool DeviceStatusStore::parseMemory(JsonObjectConst object,
                                    MemoryStatus &target, String &error) {
  if (object.isNull()) {
    error = "missing system.memory";
    return false;
  }
  uint64_t totalMB = 0;
  uint64_t usedMB = 0;
  if (!requireUint64(object, "totalMB", totalMB, error) ||
      !requireUint64(object, "usedMB", usedMB, error)) {
    return false;
  }

  constexpr uint64_t kBytesPerMB = 1024ULL * 1024ULL;
  target.totalBytes = totalMB * kBytesPerMB;
  target.usedBytes = usedMB * kBytesPerMB;
  target.availableBytes = target.totalBytes > target.usedBytes
                              ? target.totalBytes - target.usedBytes
                              : 0;
  return requireFloat(object, "usedPercent", target.usedPercent, error);
}

bool DeviceStatusStore::parseDisk(JsonObjectConst object, DiskStatus &target,
                                  String &error) {
  if (object.isNull()) {
    error = "missing system.disk";
    return false;
  }

  // 第一版只同步系统盘，路径固定为根目录。后续多盘支持可扩展为数组。
  copyText(target.path, sizeof(target.path), "/");
  uint64_t totalMB = 0;
  uint64_t usedMB = 0;
  if (!requireUint64(object, "totalMB", totalMB, error) ||
      !requireUint64(object, "usedMB", usedMB, error)) {
    return false;
  }

  constexpr uint64_t kBytesPerMB = 1024ULL * 1024ULL;
  target.totalBytes = totalMB * kBytesPerMB;
  target.usedBytes = usedMB * kBytesPerMB;
  target.freeBytes = target.totalBytes > target.usedBytes
                         ? target.totalBytes - target.usedBytes
                         : 0;
  return requireFloat(object, "usedPercent", target.usedPercent, error);
}

bool DeviceStatusStore::parsePower(JsonObjectConst object, PowerStatus &target,
                                   String &error) {
  if (object.isNull()) {
    error = "missing system.power";
    return false;
  }
  if (!object["available"].is<bool>()) {
    error = "missing or invalid system.power.available";
    return false;
  }

  target.available = object["available"].as<bool>();
  target.hasPercent = object["percent"].is<int>();
  if (target.hasPercent) {
    target.percent = object["percent"].as<int>();
  }
  target.hasCharging = object["charging"].is<bool>();
  if (target.hasCharging) {
    target.charging = object["charging"].as<bool>();
  }
  target.hasOnBattery = object["onBattery"].is<bool>();
  if (target.hasOnBattery) {
    target.onBattery = object["onBattery"].as<bool>();
  }

  const char *state = object["state"].as<const char *>();
  const char *source = object["source"].as<const char *>();
  copyText(target.state, sizeof(target.state), state == nullptr ? "" : state);
  copyText(target.source, sizeof(target.source), source == nullptr ? "" : source);
  return true;
}

bool DeviceStatusStore::requireUint64(JsonObjectConst object, const char *key,
                                      uint64_t &target, String &error) {
  if (!object[key].is<uint64_t>()) {
    error = String("missing or invalid ") + key;
    return false;
  }
  target = object[key].as<uint64_t>();
  return true;
}

bool DeviceStatusStore::requireFloat(JsonObjectConst object, const char *key,
                                     float &target, String &error) {
  if (!object[key].is<float>() && !object[key].is<double>() &&
      !object[key].is<int>()) {
    error = String("missing or invalid ") + key;
    return false;
  }
  target = object[key].as<float>();
  return true;
}

void DeviceStatusStore::copyText(char *target, size_t targetSize,
                                 const char *value) {
  if (targetSize == 0) {
    return;
  }
  strncpy(target, value, targetSize - 1);
  target[targetSize - 1] = '\0';
}
