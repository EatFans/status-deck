#include "DeviceStatusStore.h"

#include <cstring>

bool DeviceStatusStore::updateFromStatusPayload(JsonVariantConst payload,
                                                 String &error) {
  JsonObjectConst system = payload["system"].as<JsonObjectConst>();
  if (system.isNull()) {
    error = "missing payload.system";
    return false;
  }

  // 先解析到临时副本。任何字段出错时直接返回，保留屏幕正在展示的旧状态。
  SystemStatus next = status_;
  if (!parseMemory(system["memory"].as<JsonObjectConst>(), next.memory, error) ||
      !parseDisk(system["disk"].as<JsonObjectConst>(), next.disk, error) ||
      !parsePower(system["power"].as<JsonObjectConst>(), next.power, error)) {
    return false;
  }

  next.valid = true;
  next.updatedAtMs = millis();
  status_ = next;
  return true;
}

const SystemStatus &DeviceStatusStore::current() const { return status_; }

bool DeviceStatusStore::parseMemory(JsonObjectConst object,
                                    MemoryStatus &target, String &error) {
  if (object.isNull()) {
    error = "missing system.memory";
    return false;
  }
  return requireUint64(object, "totalBytes", target.totalBytes, error) &&
         requireUint64(object, "usedBytes", target.usedBytes, error) &&
         requireUint64(object, "availableBytes", target.availableBytes, error) &&
         requireFloat(object, "usedPercent", target.usedPercent, error);
}

bool DeviceStatusStore::parseDisk(JsonObjectConst object, DiskStatus &target,
                                  String &error) {
  if (object.isNull()) {
    error = "missing system.disk";
    return false;
  }

  const char *path = object["path"].as<const char *>();
  if (path == nullptr) {
    error = "missing or invalid system.disk.path";
    return false;
  }

  copyText(target.path, sizeof(target.path), path);
  return requireUint64(object, "totalBytes", target.totalBytes, error) &&
         requireUint64(object, "usedBytes", target.usedBytes, error) &&
         requireUint64(object, "freeBytes", target.freeBytes, error) &&
         requireFloat(object, "usedPercent", target.usedPercent, error);
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
