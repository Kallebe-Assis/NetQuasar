package snmpmibhints

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpprofile"
)

type Result struct {
	Applied      bool     `json:"applied"`
	Folder       string   `json:"folder,omitempty"`
	FilesScanned int      `json:"files_scanned"`
	Candidates   int      `json:"candidates"`
	Matched      int      `json:"matched"`
	Notes        []string `json:"notes,omitempty"`
}

var oidRe = regexp.MustCompile(`\.?\d+(?:\.\d+){5,}`)

// ApplyFromFolder lê txt/csv do caminho e tenta inferir OIDs de CPU/MEM/TEMP/UPTIME.
func ApplyFromFolder(folder string, rows []probing.SNMPVar, profile *snmpprofile.CollectProfile) Result {
	res := Result{Folder: strings.TrimSpace(folder)}
	if profile == nil || strings.TrimSpace(folder) == "" {
		return res
	}
	path := strings.TrimSpace(folder)
	if !filepath.IsAbs(path) {
		if wd, err := os.Getwd(); err == nil {
			path = filepath.Join(wd, path)
		}
	}
	if st, err := os.Stat(path); err != nil || !st.IsDir() {
		res.Notes = append(res.Notes, "pasta de MIB inválida ou inexistente")
		return res
	}

	exists := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		oid := cleanOID(r.OID)
		if oid == "" {
			continue
		}
		exists[oid] = struct{}{}
	}
	match := func(oid string) bool {
		oid = cleanOID(oid)
		if oid == "" {
			return false
		}
		if _, ok := exists[oid]; ok {
			return true
		}
		prefix := oid + "."
		for k := range exists {
			if strings.HasPrefix(k, prefix) {
				return true
			}
		}
		return false
	}

	var cpuC, tempC, upC, memUsedC, memSizeC []string
	_ = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(p))
		if ext != ".txt" && ext != ".csv" {
			return nil
		}
		res.FilesScanned++
		b, e := os.ReadFile(p)
		if e != nil {
			return nil
		}
		text := string(b)
		lines := strings.Split(text, "\n")
		for _, line := range lines {
			low := strings.ToLower(line)
			m := oidRe.FindAllString(line, -1)
			if len(m) == 0 {
				continue
			}
			for _, rawOID := range m {
				oid := cleanOID(rawOID)
				res.Candidates++
				switch {
				case strings.Contains(low, "uptime"):
					upC = appendUnique(upC, oid)
				case strings.Contains(low, "temperature") || strings.Contains(low, "temp"):
					tempC = appendUnique(tempC, oid)
				case strings.Contains(low, "cpu") || strings.Contains(low, "processor"):
					cpuC = appendUnique(cpuC, oid)
				case strings.Contains(low, "memory") && (strings.Contains(low, "used") || strings.Contains(low, "avail") || strings.Contains(low, "free")):
					memUsedC = appendUnique(memUsedC, oid)
				case strings.Contains(low, "memory") && (strings.Contains(low, "size") || strings.Contains(low, "total")):
					memSizeC = appendUnique(memSizeC, oid)
				}
			}
		}
		return nil
	})

	applyFirst := func(candidates []string, set func(string), note string) {
		for _, c := range candidates {
			if match(c) {
				set(c)
				res.Matched++
				res.Applied = true
				res.Notes = append(res.Notes, note+": "+c)
				return
			}
		}
	}
	applyFirst(upC, func(v string) { profile.UptimeOID = v }, "mib uptime")
	applyFirst(cpuC, func(v string) { profile.CPUPrimaryOID = v }, "mib cpu")
	applyFirst(tempC, func(v string) { profile.TempPrimaryOID = v }, "mib temp")
	applyFirst(memUsedC, func(v string) { profile.MemoryUsedOID = v }, "mib memory_used")
	applyFirst(memSizeC, func(v string) { profile.MemorySizeOID = v }, "mib memory_size")

	return res
}

func cleanOID(v string) string {
	v = strings.TrimSpace(v)
	return strings.TrimLeft(v, ".")
}

func appendUnique(list []string, v string) []string {
	v = cleanOID(v)
	if v == "" {
		return list
	}
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}
