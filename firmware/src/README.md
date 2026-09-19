# Firmware Source Layout

`app/` contains the firmware entry point and module wiring.

`ble/` contains GATT transport and chunk reassembly.

`display/` contains the ST7789 page renderer.

`status/` contains cached system and Codex payload models.

`support/` contains small hardware helpers such as the activity LED.

Headers follow the same layout under `firmware/include/`. Include module
headers by their path, for example `#include "display/StatusDisplay.h"`.
