package filter

import (
	"bufio"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

type SpamFilter struct {
	mu             sync.RWMutex
	blockedPhones  map[string]struct{}
	blockedWords   []string
	filePath       string
	lastModTime    time.Time
}

func NewSpamFilter(filePath string) *SpamFilter {
	f := &SpamFilter{
		blockedPhones: make(map[string]struct{}),
		filePath:      filePath,
	}
	f.Reload()
	go f.watchFile()
	return f
}

func (f *SpamFilter) Reload() {
	file, err := os.Open(f.filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("Error opening spam file: %v", err)
		}
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return
	}

	phones := make(map[string]struct{})
	words := []string{}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := strings.TrimSpace(parts[1])

		if key == "phone" {
			phones[val] = struct{}{}
		} else if key == "word" {
			words = append(words, strings.ToLower(val))
		}
	}

	f.mu.Lock()
	f.blockedPhones = phones
	f.blockedWords = words
	f.lastModTime = stat.ModTime()
	f.mu.Unlock()
	log.Printf("Spam filter reloaded: %d phones, %d words blocked", len(phones), len(words))
}

func (f *SpamFilter) watchFile() {
	ticker := time.NewTicker(5 * time.Second)
	for range ticker.C {
		stat, err := os.Stat(f.filePath)
		if err != nil {
			continue
		}

		f.mu.RLock()
		lastMod := f.lastModTime
		f.mu.RUnlock()

		if stat.ModTime().After(lastMod) {
			f.Reload()
		}
	}
}

func (f *SpamFilter) IsSpam(phone, message string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Check phone
	cleanPhone := strings.TrimPrefix(phone, "+")
	cleanPhone = strings.Split(cleanPhone, "@")[0] // Handle JIDs

	if _, blocked := f.blockedPhones[cleanPhone]; blocked {
		return true
	}

	// Check words
	msgLower := strings.ToLower(message)
	for _, word := range f.blockedWords {
		if strings.Contains(msgLower, word) {
			return true
		}
	}

	return false
}
