#pragma once

#include <Arduino.h>
#include <ArduinoJson.h>

/**
 * BleChunkAssembler
 *
 * 重组桌面端发送的 type=chunk 消息。
 *
 * 经典 ESP32 与 macOS 的单次 GATT 写入长度有限。system.update、codex.update 等
 * 独立模块消息通常足够短；某个单独模块未来变大时，桌面端会将原始 JSON 按顺序
 * 拆成多个 chunk。本类校验 ref、index、total 和总长度，只在收齐最后一块时返回
 * 完整原始 JSON。
 */
class BleChunkAssembler {
public:
  /**
   * 接收一个 chunk 的 payload。
   *
   * 返回 true 表示该 chunk 有效；completeMessage 为空说明仍在等待后续分片，非空
   * 时表示已重组完成，可直接交给原有业务消息处理逻辑。返回 false 时当前
   * 传输会被清空，调用方应回复 invalid_chunk 错误。
   */
  bool append(JsonObjectConst payload, String &completeMessage, String &error);

private:
  void reset();

  // 目前每个业务域单独传输，1024 bytes 足够容纳未来一段时间内的单模块快照，
  // 同时让后续 ArduinoJson 解析保持在安全的任务栈预算内。
  static constexpr size_t kMaxMessageBytes = 1024;
  static constexpr int kMaxChunkCount = 32;

  String ref_;
  String buffer_;
  int expectedIndex_ = 0;
  int total_ = 0;
};
