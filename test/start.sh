#!/bin/sh

# 创建数组存储所有后台进程的PID
pids=()

# 需要下载gost  
# 地址：https://github.com/ginuerzh/gost
# 下载后，将gost可执行文件放到当前目录
# 添加执行权限：chmod +x gost

# 启动http代理
echo "start http proxy"
gost -L http://:9001 &
pids+=($!)

# 启动https代理
echo "start https proxy"
gost -L https://:9002 &
pids+=($!)

# 启动http2代理
echo "start http2 proxy"
gost -L http2://:9003 &
pids+=($!)

# 启动socks4代理
echo "start socks4 proxy"
gost -L socks4a://:9004 &
pids+=($!)

# 启动socks5代理
echo "start socks5 proxy"
gost -L socks5://:9005 &
pids+=($!)

# 启动gost混合代理
echo "start all proxy"
gost -L :9000 &
pids+=($!)

echo "start all proxy done"

# 清理函数
cleanup() {
    echo "正在停止所有代理服务..."
    for pid in "${pids[@]}"; do
        kill $pid 2>/dev/null
    done
    echo "所有代理服务已停止"
    exit 0
}

# 注册清理函数，在脚本接收到SIGTERM或SIGINT信号时执行
trap cleanup SIGTERM SIGINT

# 无限循环保持脚本运行
while true; do
    sleep 1
done
