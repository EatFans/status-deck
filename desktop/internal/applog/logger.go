package applog

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
)

var (
	mu      sync.Mutex
	logger  *log.Logger
	logFile *os.File
	logPath string
)

// Init 初始化应用日志文件。
//
// 状态栏应用没有一个长期可见的终端窗口，所以调试信息统一写入文件。
// 用户点击“调试日志”菜单时，会打开系统终端并 tail -f 这个文件。
func Init() error {
	mu.Lock()
	defer mu.Unlock()

	if logger != nil {
		return nil
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return fmt.Errorf("get user cache dir: %w", err)
	}

	dir := filepath.Join(cacheDir, "Status Deck", "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	logPath = filepath.Join(dir, "status-deck.log")
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	logFile = file
	logger = log.New(file, "", log.LstdFlags|log.Lmicroseconds)
	logger.Println("Status Deck 日志已启动")
	return nil
}

// Path 返回当前日志文件路径。
func Path() string {
	mu.Lock()
	defer mu.Unlock()
	return logPath
}

// Printf 写入一条格式化日志。
func Printf(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()

	if logger == nil {
		return
	}

	logger.Printf(format, args...)
}

// Println 写入一条普通日志。
func Println(args ...any) {
	mu.Lock()
	defer mu.Unlock()

	if logger == nil {
		return
	}

	logger.Println(args...)
}

// Close 关闭日志文件。
func Close() error {
	mu.Lock()
	defer mu.Unlock()

	if logFile == nil {
		return nil
	}

	logger.Println("Status Deck 日志已关闭")
	err := logFile.Close()
	logFile = nil
	logger = nil
	return err
}
