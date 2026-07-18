package service

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
)

type InstallResult struct {
	Path     string
	Platform string
	Loaded   bool
}

func Install(name string, args []string) (InstallResult, error) {
	executable, err := os.Executable()
	if err != nil {
		return InstallResult{}, fmt.Errorf("resolve executable: %w", err)
	}
	executable, _ = filepath.Abs(executable)
	switch runtime.GOOS {
	case "darwin":
		return installLaunchd(name, executable, args)
	case "linux":
		return installSystemd(name, executable, args)
	case "windows":
		return installWindows(name, executable, args)
	default:
		return InstallResult{}, fmt.Errorf("service installation is not supported on %s", runtime.GOOS)
	}
}

func installLaunchd(name, executable string, args []string) (InstallResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return InstallResult{}, err
	}
	path := filepath.Join(home, "Library", "LaunchAgents", label(name)+".plist")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return InstallResult{}, err
	}
	arguments := append([]string{executable}, args...)
	var argumentXML strings.Builder
	for _, argument := range arguments {
		fmt.Fprintf(&argumentXML, "\n        <string>%s</string>", xmlEscape(argument))
	}
	content, err := render(launchdTemplate, map[string]string{
		"Label": label(name), "Arguments": argumentXML.String(),
		"Stdout": filepath.Join(home, "Library", "Logs", name+".log"),
		"Stderr": filepath.Join(home, "Library", "Logs", name+".error.log"),
	})
	if err != nil {
		return InstallResult{}, err
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return InstallResult{}, err
	}
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	_ = exec.Command("launchctl", "bootout", domain+"/"+label(name)).Run()
	if output, err := exec.Command("launchctl", "bootstrap", domain, path).CombinedOutput(); err != nil {
		return InstallResult{Path: path, Platform: "launchd"}, fmt.Errorf("launchctl bootstrap: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return InstallResult{Path: path, Platform: "launchd", Loaded: true}, nil
}

func installSystemd(name, executable string, args []string) (InstallResult, error) {
	config, err := os.UserConfigDir()
	if err != nil {
		return InstallResult{}, err
	}
	path := filepath.Join(config, "systemd", "user", name+".service")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return InstallResult{}, err
	}
	command := systemdQuote(executable)
	for _, argument := range args {
		command += " " + systemdQuote(argument)
	}
	content, err := render(systemdTemplate, map[string]string{
		"Description": description(name), "Command": command,
	})
	if err != nil {
		return InstallResult{}, err
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return InstallResult{}, err
	}
	if output, err := exec.Command("systemctl", "--user", "daemon-reload").CombinedOutput(); err != nil {
		return InstallResult{Path: path, Platform: "systemd"}, fmt.Errorf("systemctl daemon-reload: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if output, err := exec.Command("systemctl", "--user", "enable", "--now", name+".service").CombinedOutput(); err != nil {
		return InstallResult{Path: path, Platform: "systemd"}, fmt.Errorf("systemctl enable: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return InstallResult{Path: path, Platform: "systemd", Loaded: true}, nil
}

func installWindows(name, executable string, args []string) (InstallResult, error) {
	command := `"` + executable + `"`
	for _, argument := range args {
		command += ` "` + strings.ReplaceAll(argument, `"`, `\"`) + `"`
	}
	serviceName := "ClipHub-" + name
	_ = exec.Command("sc.exe", "stop", serviceName).Run()
	_ = exec.Command("sc.exe", "delete", serviceName).Run()
	output, err := exec.Command("sc.exe", "create", serviceName, "start=", "auto", "binPath=", command, "DisplayName=", description(name)).CombinedOutput()
	if err != nil {
		return InstallResult{Path: serviceName, Platform: "windows-service"}, fmt.Errorf("sc.exe create (run as Administrator): %w: %s", err, strings.TrimSpace(string(output)))
	}
	if output, err = exec.Command("sc.exe", "start", serviceName).CombinedOutput(); err != nil {
		return InstallResult{Path: serviceName, Platform: "windows-service"}, fmt.Errorf("sc.exe start: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return InstallResult{Path: serviceName, Platform: "windows-service", Loaded: true}, nil
}

func render(source string, data any) ([]byte, error) {
	tmpl, err := template.New("service").Parse(source)
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	if err := tmpl.Execute(&output, data); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func label(name string) string { return "com.thalys.cliphub." + name }

func description(name string) string {
	if name == "cliphub" {
		return "ClipHub tailnet clipboard hub"
	}
	return "ClipHub clipboard synchronization agent"
}

func systemdQuote(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, `%`, `%%`).Replace(value) + `"`
}

func xmlEscape(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;").Replace(value)
}

const launchdTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key><string>{{.Label}}</string>
    <key>ProgramArguments</key><array>{{.Arguments}}
    </array>
    <key>RunAtLoad</key><true/>
    <key>KeepAlive</key><true/>
    <key>StandardOutPath</key><string>{{.Stdout}}</string>
    <key>StandardErrorPath</key><string>{{.Stderr}}</string>
</dict>
</plist>
`

const systemdTemplate = `[Unit]
Description={{.Description}}
After=network-online.target
Wants=network-online.target

[Service]
ExecStart={{.Command}}
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
`
