#!/bin/bash

cd "$(dirname "$0")"

./stop_bot_on_rpi.sh
./build_for_rpi.sh
./copy_bin_to_rpi.sh
./start_bot_on_rpi.sh
./view_logs_rpi.sh