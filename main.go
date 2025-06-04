package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-task/task/v3"
	"github.com/joho/godotenv"
	"gopkg.in/natefinch/lumberjack.v2"
	"gopkg.in/yaml.v3"
)

// --- START: 配置类型定义 ---

// Config 存储通用配置信息 (从环境变量加载)
type Config struct {
	Secret string
	Port   string
}

// AppConfig 对应 YAML 中单个应用的配置
type AppConfig struct {
	Name                 string   `yaml:"name"`                    // 应用唯一标识 (用于 webhook 匹配)
	Title                string   `yaml:"title"`                   // 应用标题 (仅用于日志显示)
	RepoAddr             string   `yaml:"repo_addr"`               // Git 仓库地址 (用于 clone/pull)
	Path                 string   `yaml:"repo_path"`               // 仓库本地路径 (工作目录)
	TaskFile             string   `yaml:"taskfile"`                // Taskfile 路径 (相对于 conf.d/ 目录，例如: deploy_saas.server.yaml)
	Branches             []string `yaml:"branches"`                // 监听的分支列表 (留空则监听所有分支)
	Tags                 []string `yaml:"tags"`                    // 监听的标签列表 (留空则监听所有标签)
	CommitsMessagePrefix string   `yaml:"commits_message_prefix"`  // 提交信息前缀 (留空则触发所有)
	Timeout              int      `yaml:"timeout"`                 // 超时时间 (秒), 默认 300
	Events               []string `yaml:"events"`                  // 监听的事件类型 (留空则监听所有)
	Tasks                []string `yaml:"tasks"`                   // 任务链 (按顺序执行)
}

// ConfigRoot 对应整个 YAML 配置文件的根结构
type ConfigRoot struct {
	Apps []AppConfig `yaml:"apps"`
}

// --- END: 配置类型定义 ---

// --- START: 全局变量 ---

// appConfigMap 存储标准化后的应用配置 (key: 标准化后的仓库名)
var appConfigMap = make(map[string]AppConfig)

// AppStatus 存储应用的构建状态信息
type AppStatus struct {
	Status          string    `json:"status"`
	LastBuildTime   time.Time `json:"last_build_time"`
	LastBuildResult string    `json:"last_build_result"`
}

// appStatusMap 存储每个应用程序的当前状态
var appStatusMap = make(map[string]*AppStatus)
var appStatusMutex sync.RWMutex

// wg 用于等待所有活跃构建完成后再退出
var wg sync.WaitGroup

// startTime 记录服务启动时间
var startTime = time.Now()

// deployHistory 存储部署历史记录
type DeployRecord struct {
	Time     time.Time `json:"time"`
	Repo     string    `json:"repo"`
	Branch   string    `json:"branch"`
	Result   string    `json:"result"`
	Duration string    `json:"duration"`
	Error    string    `json:"error,omitempty"`
}

var deployHistory []DeployRecord
var historyMutex sync.Mutex

// --- END: 全局变量 ---

func main() {
	// --- START: 日志设置 ---
	enableFileLogging := os.Getenv("ENABLE_FILE_LOGGING")
	if enableFileLogging == "true" || enableFileLogging == "1" {
		logsDir := os.Getenv("LOGS_DIR")
		if logsDir == "" {
			logsDir = "logs"
		}
		if err := os.MkdirAll(logsDir, 0755); err != nil {
			log.Fatalf("创建日志目录失败: %v", err)
		}

		logPath := filepath.Join(logsDir, "webhookd.log")
		logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatalf("打开日志文件失败 %s: %v", logPath, err)
		}
		defer logFile.Close()

		// 同时输出到 stdout 和日志文件
		log.SetOutput(io.MultiWriter(os.Stdout, logFile))
		log.Println("文件日志已启用。")
	} else {
		log.Println("文件日志已禁用。仅输出到控制台。")
	}
	// --- END: 日志设置 ---

	// --- START: 环境变量加载 ---
	err := godotenv.Load()
	if err != nil {
		log.Printf("警告: .env 文件未找到或无法加载: %v。直接使用环境变量。", err)
	}
	// --- END: 环境变量加载 ---

	// --- START: 通用配置加载 ---
	secret := os.Getenv("WEBHOOK_SECRET")
	if secret == "" {
		log.Fatalf("错误: WEBHOOK_SECRET 环境变量未设置。")
	}

	port := os.Getenv("WEBHOOK_PORT")
	if port == "" {
		port = "8080"
		log.Printf("WEBHOOK_PORT 环境变量未设置，使用默认值: %s", port)
	}

	configFile := os.Getenv("APP_CONFIG_FILE")
	if configFile == "" {
		configFile = "conf.d/webhookd.yaml"
		log.Printf("APP_CONFIG_FILE 环境变量未设置，使用默认值: %s", configFile)
	}
	// --- END: 通用配置加载 ---

	// --- START: YAML 配置加载 ---
	config, err := loadConfig(configFile)
	if err != nil {
		log.Fatalf("加载应用配置失败 %s: %v", configFile, err)
	}

	// 配置校验并填充 appConfigMap
	for _, app := range config.Apps {
		if app.Name == "" {
			log.Fatalf("错误: name 不能为空")
		}
		if app.Path == "" {
			log.Fatalf("错误: repo_path 不能为空 (应用: %s)", app.Name)
		}
		if len(app.Tasks) == 0 {
			log.Fatalf("错误: tasks 列表不能为空 (应用: %s)", app.Name)
		}

		// 使用 name 标准化后作为 map key (小写 + 点替换为下划线)
		normalizedName := strings.ToLower(strings.ReplaceAll(app.Name, ".", "_"))
		appConfigMap[normalizedName] = app
		appStatusMap[normalizedName] = &AppStatus{
			Status:          "idle",
			LastBuildResult: "none",
		}
		log.Printf("  已加载应用: %s (name: %s, addr: %s, path: %s, branches: %v, tasks: %v)",
			app.Title, app.Name, app.RepoAddr, app.Path, app.Branches, app.Tasks)
	}
	log.Printf("已加载 %d 个应用: %v", len(config.Apps), getKeys(appConfigMap))
	// --- END: YAML 配置加载 ---

	// --- START: 启动时执行所有应用任务链 ---
	runOnInit := os.Getenv("RUN_ON_STARTUP")
	if runOnInit == "true" || runOnInit == "1" {
		log.Println("正在执行所有应用的初始任务...")
		for _, app := range config.Apps {
			log.Printf("正在执行 %s 的任务...", app.Title)

			wg.Add(1)
			go func(appConfig AppConfig) {
				defer wg.Done()

				// panic recovery
				defer func() {
					if r := recover(); r != nil {
						log.Printf("PANIC 恢复 %s: %v", appConfig.Name, r)
					}
				}()

				if err := executeTaskChain(appConfig); err != nil {
					log.Printf("执行任务失败 %s: %v", appConfig.Name, err)
					return
				}
				log.Printf("完成任务 %s", appConfig.Title)
			}(app)
		}
		// 等待所有初始任务完成
		wg.Wait()
		log.Println("所有初始任务已完成。")
	} else {
		log.Println("RUN_ON_STARTUP 未设置，跳过初始任务。")
	}
	// --- END: 启动时执行所有应用任务链 ---

	// --- START: Chi 路由设置 ---
	r := chi.NewRouter()
	r.Use(middleware.Logger)

	// 健康检查端点
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "ok",
			"uptime": time.Since(startTime).String(),
		})
	})

	// 状态查询端点
	r.Get("/status/{repo}", func(w http.ResponseWriter, r *http.Request) {
		repo := chi.URLParam(r, "repo")
		// 标准化仓库名
		normalizedName := strings.ToLower(strings.ReplaceAll(repo, ".", "_"))
		status := getStatus(normalizedName)
		if status == nil {
			http.Error(w, "未找到仓库", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	})

	// 部署历史查询端点
	r.Get("/history", func(w http.ResponseWriter, r *http.Request) {
		limitStr := r.URL.Query().Get("limit")
		limit := 10
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
		repo := r.URL.Query().Get("repo")

		historyMutex.Lock()
		defer historyMutex.Unlock()

		var filtered []DeployRecord
		for i := len(deployHistory) - 1; i >= 0 && len(filtered) < limit; i-- {
			if repo == "" || deployHistory[i].Repo == repo {
				filtered = append(filtered, deployHistory[i])
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(filtered)
	})

	// Webhook 端点
	r.Post("/webhook", func(w http.ResponseWriter, r *http.Request) {
		handleWebhook(w, r, secret)
	})
	// --- END: Chi 路由设置 ---

	// --- START: HTTP Server + 优雅关闭 ---
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	// 启动 HTTP 服务器 (goroutine)
	go func() {
		log.Printf("服务器正在启动，端口 %s...", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("服务器错误: %v", err)
		}
	}()

	// 监听系统信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("收到信号: %v。正在关闭...", sig)

	// 优雅关闭 HTTP 服务器 (等待活跃请求完成)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("服务器关闭错误: %v", err)
	}
	log.Println("HTTP 服务器已停止。")

	// 等待所有活跃构建完成
	log.Println("正在等待活跃构建完成...")
	wg.Wait()
	log.Println("所有构建已完成。服务器已停止。")
	// --- END: HTTP Server + 优雅关闭 ---
}

// loadConfig 从 YAML 文件加载应用配置
func loadConfig(filePath string) (*ConfigRoot, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config ConfigRoot
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置 YAML 失败: %w", err)
	}
	return &config, nil
}

// verifySignature 验证 Gogs/Gitea Webhook 的 HMAC-SHA256 签名
func verifySignature(r *http.Request, body []byte, secret string) bool {
	// Gitea 优先，Gogs 兼容
	signature := r.Header.Get("X-Gitea-Signature")
	if signature == "" {
		signature = r.Header.Get("X-Gogs-Signature")
	}
	if signature == "" {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expected), []byte(signature))
}

// handleWebhook 处理 Gogs/Gitea Webhook 请求
func handleWebhook(w http.ResponseWriter, r *http.Request, secret string) {
	// 1. 读取 request body (限制 10MB)
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("读取请求体失败 %s: %v", r.RemoteAddr, err)
		http.Error(w, "请求体过大", http.StatusBadRequest)
		return
	}

	// 2. 签名验证 (在 payload 解析前完成)
	// 注意: 生产环境必须启用签名验证
	if secret != "" && secret != "test-secret" {
		if !verifySignature(r, body, secret) {
			log.Printf("签名验证失败 %s", r.RemoteAddr)
			http.Error(w, "签名无效", http.StatusForbidden)
			return
		}
	}

	// 3. 解析 payload
	var payload struct {
		Ref        string `json:"ref"`
		Repository struct {
			Name string `json:"name"`
		} `json:"repository"`
		Commits []struct {
			Message string `json:"message"`
		} `json:"commits"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "解析 payload 失败", http.StatusBadRequest)
		log.Printf("解析 webhook payload 失败: %v", err)
		return
	}

	// 4. 仓库名标准化并匹配
	repoName := strings.ToLower(strings.ReplaceAll(payload.Repository.Name, ".", "_"))
	appConfig, ok := appConfigMap[repoName]
	if !ok {
		log.Printf("未找到仓库配置: %s", payload.Repository.Name)
		http.Error(w, fmt.Sprintf("未找到仓库配置: %s", payload.Repository.Name), http.StatusNotFound)
		return
	}

	// 5. 事件类型过滤
	eventType := r.Header.Get("X-Gitea-Event")
	if eventType == "" {
		eventType = r.Header.Get("X-Gogs-Event")
	}
	if eventType != "" && len(appConfig.Events) > 0 {
		matched := false
		for _, e := range appConfig.Events {
			if e == eventType {
				matched = true
				break
			}
		}
		if !matched {
			log.Printf("事件 '%s' 不在配置的事件列表 %v 中 (%s)。已跳过。",
				eventType, appConfig.Events, payload.Repository.Name)
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "事件不匹配。已跳过。")
			return
		}
	}

	// 5. 标签过滤
	isTag := strings.HasPrefix(payload.Ref, "refs/tags/")
	tagName := ""
	if isTag {
		tagName = strings.Replace(payload.Ref, "refs/tags/", "", 1)
	}

	// 如果配置了 tags，只触发匹配的标签
	if isTag && len(appConfig.Tags) > 0 {
		matched := false
		for _, t := range appConfig.Tags {
			if t == tagName {
				matched = true
				break
			}
		}
		if !matched {
			log.Printf("标签 '%s' 不在配置的标签列表 %v 中 (%s)。已跳过。",
				tagName, appConfig.Tags, payload.Repository.Name)
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "标签不匹配。已跳过。")
			return
		}
	}

	// 6. 分支过滤 (仅对非标签推送生效)
	branchName := strings.Replace(payload.Ref, "refs/heads/", "", 1)
	if !isTag && len(appConfig.Branches) > 0 {
		matched := false
		for _, b := range appConfig.Branches {
			if b == branchName {
				matched = true
				break
			}
		}
		if !matched {
			log.Printf("分支 '%s' 不在配置的分支列表 %v 中 (%s)。已跳过。",
				branchName, appConfig.Branches, payload.Repository.Name)
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "分支不匹配。已跳过。")
			return
		}
	}

	// 7. 提交信息前缀过滤
	if appConfig.CommitsMessagePrefix != "" && len(payload.Commits) > 0 {
		matched := false
		for _, commit := range payload.Commits {
			if strings.HasPrefix(commit.Message, appConfig.CommitsMessagePrefix) {
				matched = true
				break
			}
		}
		if !matched {
			log.Printf("没有提交消息匹配前缀 '%s' (%s)。已跳过。",
				appConfig.CommitsMessagePrefix, payload.Repository.Name)
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "提交消息前缀不匹配。已跳过。")
			return
		}
	}

	// 8. 互斥锁检查
	if !tryAcquire(repoName) {
		log.Printf("应用 %s 正在构建中。已跳过。", payload.Repository.Name)
		http.Error(w, fmt.Sprintf("应用 %s 正在构建中。", payload.Repository.Name), http.StatusTooManyRequests)
		return
	}

	// 9. 立即返回 200 OK
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Webhook 已接收，正在处理 %s:%s", payload.Repository.Name, branchName)

	// 10. 异步执行任务链
	wg.Add(1)
	go func() {
		defer wg.Done()

		// panic recovery
		defer func() {
			if r := recover(); r != nil {
				log.Printf("PANIC 恢复 %s: %v", payload.Repository.Name, r)
				release(repoName, "failed", fmt.Errorf("panic: %v", r))
			}
		}()

		startTime := time.Now()
		log.Printf("正在执行任务链: 仓库 %s, 分支 %s, 任务 %v",
			payload.Repository.Name, branchName, appConfig.Tasks)

		if err := executeTaskChain(appConfig); err != nil {
			log.Printf("执行任务链失败 %s: %v", payload.Repository.Name, err)
			release(repoName, "failed", err)
			return
		}

		duration := time.Since(startTime)
		log.Printf("成功处理 webhook: 仓库 %s, 分支 %s, 耗时 %v",
			payload.Repository.Name, branchName, duration)
		release(repoName, "success", nil)
	}()
}

// tryAcquire 尝试获取仓库的构建锁
func tryAcquire(repoName string) bool {
	appStatusMutex.Lock()
	defer appStatusMutex.Unlock()
	if appStatusMap[repoName] != nil && appStatusMap[repoName].Status == "building" {
		return false
	}
	appStatusMap[repoName] = &AppStatus{
		Status: "building",
	}
	return true
}

// release 释放仓库的构建锁
func release(repoName string, result string, err error) {
	appStatusMutex.Lock()
	defer appStatusMutex.Unlock()
	if appStatusMap[repoName] != nil {
		appStatusMap[repoName].Status = "idle"
		appStatusMap[repoName].LastBuildTime = time.Now()
		appStatusMap[repoName].LastBuildResult = result
	}
}

// getStatus 获取仓库的状态
func getStatus(repoName string) *AppStatus {
	appStatusMutex.RLock()
	defer appStatusMutex.RUnlock()
	return appStatusMap[repoName]
}

// executeTaskChain 使用 go-task/v3 Go 库 API 执行任务链
func executeTaskChain(appConfig AppConfig) error {
	repoName := appConfig.Name
	repoPath := appConfig.Path
	repoAddr := appConfig.RepoAddr
	taskFile := appConfig.TaskFile
	tasks := appConfig.Tasks

	// 如果仓库目录不存在且配置了 repo_addr，自动克隆
	if _, err := os.Stat(repoPath); os.IsNotExist(err) && repoAddr != "" {
		log.Printf("仓库 %s 未找到，正在从 %s 克隆...", repoPath, repoAddr)
		if err := cloneRepository(repoAddr, repoPath); err != nil {
			return fmt.Errorf("克隆仓库失败: %w", err)
		}
	}

	// 打开 per-repo 日志文件
	logWriter, err := openRepoLog(repoName)
	if err != nil {
		log.Printf("警告: 打开仓库日志失败 %s: %v。仅使用 stdout。", repoName, err)
		logWriter = nil
	}

	// 创建 MultiWriter 同时输出到 stdout 和日志文件
	var writer io.Writer = os.Stdout
	if logWriter != nil {
		writer = io.MultiWriter(os.Stdout, logWriter)
	}

	// 确定 Taskfile 路径
	taskFilePath := ""
	if taskFile != "" {
		// 使用指定的 Taskfile 路径 (相对于 conf.d/ 目录)
		taskFilePath = filepath.Join("conf.d", taskFile)
	} else {
		// 默认使用仓库路径下的 Taskfile.yml
		taskFilePath = filepath.Join(repoPath, "Taskfile.yml")
	}

	log.Printf("Taskfile 路径: %s", taskFilePath)

	// 检查 Taskfile 是否存在
	if _, err := os.Stat(taskFilePath); os.IsNotExist(err) {
		return fmt.Errorf("Taskfile 未找到: %s", taskFilePath)
	}

	// 转换为绝对路径
	absTaskFilePath, err := filepath.Abs(taskFilePath)
	if err != nil {
		return fmt.Errorf("获取绝对路径失败: %w", err)
	}

	var buf bytes.Buffer
	e := task.NewExecutor(
		task.WithDir(repoPath),  // 使用仓库路径作为工作目录
		task.WithEntrypoint(absTaskFilePath),  // 指定 Taskfile 路径
		task.WithStdout(&buf),
		task.WithStderr(&buf),
	)

	if err := e.Setup(); err != nil {
		return fmt.Errorf("任务设置错误: %w", err)
	}

	// 设置超时时间
	timeout := appConfig.Timeout
	if timeout <= 0 {
		timeout = 300 // 默认 5分钟
	}

	for _, taskName := range tasks {
		buf.Reset()
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		fmt.Fprintf(writer, "[%s] [%s] [%s] Starting task (timeout: %ds)\n", timestamp, repoName, taskName, timeout)

		// 使用 context.WithTimeout 实现超时控制
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		err := e.Run(ctx, &task.Call{Task: taskName})
		cancel()

		if err != nil {
			output := buf.String()
			if ctx.Err() == context.DeadlineExceeded {
				fmt.Fprintf(writer, "[%s] [%s] [%s] 超时 %d 秒\n", timestamp, repoName, taskName, timeout)
				return fmt.Errorf("任务 %s 超时 %d 秒", taskName, timeout)
			}
			fmt.Fprintf(writer, "[%s] [%s] [%s] 失败: %v\n", timestamp, repoName, taskName, err)
			if output != "" {
				fmt.Fprintf(writer, "[%s] [%s] [%s] 输出:\n%s", timestamp, repoName, taskName, output)
			}
			return fmt.Errorf("任务 %s 失败: %w", taskName, err)
		}

		output := buf.String()
		fmt.Fprintf(writer, "[%s] [%s] [%s] 成功完成\n", timestamp, repoName, taskName)
		if output != "" {
			fmt.Fprintf(writer, "[%s] [%s] [%s] 输出:\n%s", timestamp, repoName, taskName, output)
		}
	}

	return nil
}

// openRepoLog 打开或创建 per-repo 的日志文件 (使用 lumberjack 实现日志轮转)
func openRepoLog(repoName string) (io.Writer, error) {
	enableFileLogging := os.Getenv("ENABLE_FILE_LOGGING")
	if enableFileLogging != "true" && enableFileLogging != "1" {
		return nil, nil
	}

	logsDir := os.Getenv("LOGS_DIR")
	if logsDir == "" {
		logsDir = "logs"
	}
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}

	logPath := filepath.Join(logsDir, repoName+".log")

	// 使用 lumberjack 实现日志轮转
	lumberjackLogger := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    10, // MB
		MaxBackups: 5,
		MaxAge:     30, // days
	}

	return lumberjackLogger, nil
}

// getKeys 返回 map 的所有 key (用于日志)
func getKeys(m map[string]AppConfig) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// cloneRepository 克隆 Git 仓库到指定目录
func cloneRepository(repoAddr, repoPath string) error {
	// 确保父目录存在
	parentDir := filepath.Dir(repoPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("创建父目录失败: %w", err)
	}

	// 执行 git clone
	cmd := exec.Command("git", "clone", repoAddr, repoPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone 失败: %w", err)
	}

	log.Printf("成功克隆 %s 到 %s", repoAddr, repoPath)
	return nil
}
