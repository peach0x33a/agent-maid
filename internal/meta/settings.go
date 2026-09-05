// 用户设置(含首次运行向导的结果)与仓库信息。
package meta

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Repo 项目仓库地址(footer 展示用)。
const Repo = "github.com/peach0x33a/agent-maid"

// Settings 用户级设置。
type Settings struct {
	SetupDone bool `json:"setupDone"` // 首次运行向导已完成
	Truecolor bool `json:"truecolor"` // 向导:终端支持渐变色
	Symbols   bool `json:"symbols"`   // 向导:终端支持半块/制表等特殊符号
	Plain     bool `json:"plain"`     // 符号不受支持:启用纯文本降级(无块状图标、ASCII spinner)
	NoAnim    bool `json:"no_anim"`   // 关闭动画(流光/摇晃/转轮)
	// LastTarget 上次使用的目标软件 Key;首页预选它。
	LastTarget string `json:"last_target,omitempty"`
	// BackupDays 凭据备份保留天数(1-30);未设置时默认 30。
	BackupDays *int `json:"backup_days,omitempty"`
}

// BackupDaysValue 返回备份保留天数(默认 30,上限 30,下限 1)。
func (s Settings) BackupDaysValue() int {
	d := 30
	if s.BackupDays != nil {
		d = *s.BackupDays
	}
	if d < 1 {
		d = 1
	}
	if d > 30 {
		d = 30
	}
	return d
}

// ChatGPTBackupDir 返回 ChatGPT auth.json 备份的存放目录(自有目录内)。
func ChatGPTBackupDir() string { return filepath.Join(Dir(), "chatgpt", "backups") }

// PurgeExpiredFiles 删除目录中修改时间超过 days 天的文件(用于备份定期清理)。

// SettingsPath 返回设置文件路径。
func SettingsPath() string { return filepath.Join(Dir(), "settings.json") }

// LoadSettings 读取设置;文件缺失或损坏时返回零值(SetupDone=false → 触发向导)。
func LoadSettings() Settings {
	data, err := os.ReadFile(SettingsPath())
	if err != nil {
		return Settings{}
	}
	var s Settings
	if json.Unmarshal(data, &s) != nil {
		return Settings{}
	}
	return s
}

// SaveSettings 原子落盘设置。
func SaveSettings(s Settings) error {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(SettingsPath(), append(data, '\n'), 0o600)
}
