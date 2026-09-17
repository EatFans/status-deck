#include "BleChunkAssembler.h"

#include <cstring>

bool BleChunkAssembler::append(JsonObjectConst payload, String &completeMessage,
                               String &error) {
  completeMessage = "";
  const char *ref = payload["ref"].as<const char *>();
  const char *encoding = payload["encoding"].as<const char *>();
  const char *data = payload["data"].as<const char *>();
  if (ref == nullptr || encoding == nullptr || data == nullptr ||
      !payload["index"].is<int>() || !payload["total"].is<int>()) {
    error = "invalid chunk fields";
    reset();
    return false;
  }

  const int index = payload["index"].as<int>();
  const int total = payload["total"].as<int>();
  if (strcmp(encoding, "json") != 0 || total <= 0 || total > kMaxChunkCount ||
      index < 0 || index >= total) {
    error = "invalid chunk metadata";
    reset();
    return false;
  }

  if (index == 0) {
    // 新传输必须从 0 开始；这也会主动丢弃前一次中途断开的残留数据。
    reset();
    ref_ = ref;
    total_ = total;
  }
  if (ref_ != ref || total_ != total || index != expectedIndex_) {
    error = "unexpected chunk order";
    reset();
    return false;
  }
  if (buffer_.length() + strlen(data) > kMaxMessageBytes) {
    error = "chunked message too large";
    reset();
    return false;
  }

  buffer_ += data;
  expectedIndex_++;
  if (expectedIndex_ == total_) {
    completeMessage = buffer_;
    reset();
  }
  return true;
}

void BleChunkAssembler::reset() {
  ref_ = "";
  buffer_ = "";
  expectedIndex_ = 0;
  total_ = 0;
}
