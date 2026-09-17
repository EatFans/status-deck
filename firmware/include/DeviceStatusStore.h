#pragma once

#include <Arduino.h>
#include <ArduinoJson.h>

/**
 * DeviceStatusStore
 *
 * 状态卡设备端的内存状态容器。
 *
 * BLE 回调只负责把桌面端的 system.update 解析并写进这里；显示屏 UI 不需要
 * 关心 JSON 或 BLE，之后只从 current() 读取变量并按当前页面渲染。
 *
 * 第一版仅存储系统信息：内存、磁盘和电源。后续可在 SystemStatus 同级扩展
 * aiUsage、services 等字段，而不会影响已有屏幕页面。
 */

struct MemoryStatus {
  uint64_t totalBytes = 0;
  uint64_t usedBytes = 0;
  uint64_t availableBytes = 0;
  float usedPercent = 0.0f;
};

struct CPUStatus {
  float usedPercent = 0.0f;
};

struct GPUStatus {
  bool available = false;
  float usedPercent = 0.0f;
};

struct DiskStatus {
  char path[32] = {};
  uint64_t totalBytes = 0;
  uint64_t usedBytes = 0;
  uint64_t freeBytes = 0;
  float usedPercent = 0.0f;
};

struct PowerStatus {
  bool available = false;
  bool hasPercent = false;
  int percent = 0;
  bool hasCharging = false;
  bool charging = false;
  bool hasOnBattery = false;
  bool onBattery = false;
  char state[24] = {};
  char source[16] = {};
};

struct SystemStatus {
  bool valid = false;
  uint32_t updatedAtMs = 0;
  CPUStatus cpu;
  GPUStatus gpu;
  MemoryStatus memory;
  DiskStatus disk;
  PowerStatus power;
};

class DeviceStatusStore {
public:
  /**
   * 解析 system.update 的 payload 字段，并原子性地替换当前系统状态。
   *
   * payload 格式：
   * {"cpu":{...},"memory":{...},"disk":{...},"power":{...}}
   *
   * 返回 false 表示字段缺失或类型不对；旧状态会被保留，不会被半截数据污染。
   */
  bool updateFromSystemPayload(JsonVariantConst payload, String &error);

  /**
   * 返回当前完整状态。
   *
   * 当前 BLE 处理和未来 UI 刷新都运行在 ESP32 同一程序内。若未来将 UI 放进
   * 独立 FreeRTOS 任务，应在读取和写入两侧增加临界区保护。
   */
  const SystemStatus &current() const;

private:
  bool parseMemory(JsonObjectConst object, MemoryStatus &target, String &error);
  bool parseCPU(JsonObjectConst object, CPUStatus &target, String &error);
  bool parseGPU(JsonObjectConst object, GPUStatus &target, String &error);
  bool parseDisk(JsonObjectConst object, DiskStatus &target, String &error);
  bool parsePower(JsonObjectConst object, PowerStatus &target, String &error);
  bool requireUint64(JsonObjectConst object, const char *key, uint64_t &target,
                     String &error);
  bool requireFloat(JsonObjectConst object, const char *key, float &target,
                    String &error);
  void copyText(char *target, size_t targetSize, const char *value);

  SystemStatus status_;
};
