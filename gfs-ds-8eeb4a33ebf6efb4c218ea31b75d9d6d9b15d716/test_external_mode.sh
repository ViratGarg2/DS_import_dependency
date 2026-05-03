#!/bin/bash
# Test external client mode

cd /home/gvarun01/Projects/DS_project/DS_import_dependency/gfs-ds-8eeb4a33ebf6efb4c218ea31b75d9d6d9b15d716

echo "Testing External Client Mode..."
echo "==============================="
echo ""

timeout 20 ./bin/gfs-client --config configs/external/client-config.yml <<'HEREDOC'
create exttest.txt
append exttest.txt "Hello from External Mode"
read exttest.txt 0 100
ls
exit
HEREDOC

echo ""
echo "Test completed!"
