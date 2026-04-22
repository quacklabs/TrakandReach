package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/username/trakand-reach/pkg/api"
	"github.com/playwright-community/playwright-go"
	"github.com/username/trakand-reach/pkg/db"
	"github.com/username/trakand-reach/pkg/engine"
	"github.com/username/trakand-reach/pkg/models"
)

var port int

func main() {
	var rootCmd = &cobra.Command{Use: "trakand-reach"}

	var runCmd = &cobra.Command{
		Use:   "run",
		Short: "Start the Trakand Reach engine",
		Run: func(cmd *cobra.Command, args []string) {
			home, _ := os.UserHomeDir()
			dbPath := filepath.Join(home, ".trakand_reach", "reach.db")
			repo, err := db.NewRepository(dbPath)
			if err != nil {
				log.Fatalf("DB Error: %v", err)
			}

			manager, err := engine.NewManager(repo)
			if err != nil {
				log.Fatalf("Engine Error: %v", err)
			}

			if err := manager.Start(); err != nil {
				log.Fatalf("Failed to start manager: %v", err)
			}

			server := api.NewServer(manager)
			log.Printf("Trakand Reach Go Service starting on port %d ✅", port)
			log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), server))
		},
	}
	runCmd.Flags().IntVarP(&port, "port", "p", 3000, "Port to run on")

	var installCmd = &cobra.Command{
		Use:   "install",
		Short: "Install Playwright browsers",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Installing Playwright browsers (WebKit)...")
			err := playwright.Install()
			if err != nil {
				fmt.Printf("Error installing browsers: %v\n", err)
				return
			}
			fmt.Println("Installation complete ✅")
		},
	}

	var setupCmd = &cobra.Command{
		Use:   "setup",
		Short: "Setup and start systemd service (requires sudo)",
		Run: func(cmd *cobra.Command, args []string) {
			user := os.Getenv("SUDO_USER")
			if user == "" {
				user = os.Getenv("USER")
			}
			if user == "" {
				user = "root"
			}

			cwd, _ := os.Getwd()
			execPath, _ := filepath.Abs(os.Args[0])

			serviceContent := fmt.Sprintf(`[Unit]
Description=Trakand Reach Go Engine
After=network.target

[Service]
Type=simple
User=%s
WorkingDirectory=%s
ExecStart=%s run --port %d
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
`, user, cwd, execPath, port)

			servicePath := "/etc/systemd/system/trakand-reach.service"
			fmt.Printf("Installing systemd service to %s...\n", servicePath)

			err := os.WriteFile("/tmp/trakand-reach.service", []byte(serviceContent), 0644)
			if err != nil {
				log.Fatalf("Failed to write temp service file: %v", err)
			}

			cmds := [][]string{
				{"sudo", "mv", "/tmp/trakand-reach.service", servicePath},
				{"sudo", "systemctl", "daemon-reload"},
				{"sudo", "systemctl", "enable", "trakand-reach"},
				{"sudo", "systemctl", "start", "trakand-reach"},
			}

			for _, c := range cmds {
				if err := exec.Command(c[0], c[1:]...).Run(); err != nil {
					log.Printf("Warning: command %v failed: %v", c, err)
				}
			}

			fmt.Println("Trakand Reach service installed and started successfully ✅")
		},
	}

	var uninstallCmd = &cobra.Command{
		Use:   "uninstall",
		Short: "Remove systemd service (requires sudo)",
		Run: func(cmd *cobra.Command, args []string) {
			cmds := [][]string{
				{"sudo", "systemctl", "stop", "trakand-reach"},
				{"sudo", "systemctl", "disable", "trakand-reach"},
				{"sudo", "rm", "/etc/systemd/system/trakand-reach.service"},
				{"sudo", "systemctl", "daemon-reload"},
			}
			for _, c := range cmds {
				exec.Command(c[0], c[1:]...).Run()
			}
			fmt.Println("Trakand Reach service uninstalled successfully ✅")
		},
	}

	var whatsappCmd = &cobra.Command{
		Use:   "whatsapp",
		Short: "Quick start WhatsApp Web session",
		Run: func(cmd *cobra.Command, args []string) {
			home, _ := os.UserHomeDir()
			dbPath := filepath.Join(home, ".trakand_reach", "reach.db")
			repo, _ := db.NewRepository(dbPath)
			manager, _ := engine.NewManager(repo)
			manager.Start()
			defer manager.Stop()

			session := &models.Session{
				ID: "whatsapp-default",
				DeviceInfo: models.DeviceInfo{
					UserAgent:  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
					Width:      1280,
					Height:     720,
					PixelRatio: 1.0,
				},
				LastURL: "https://web.whatsapp.com",
			}

			_, _ = manager.StartSession(session)
			fmt.Printf("WhatsApp session started. WebSocket available on port %d\n", port)

			server := api.NewServer(manager)
			log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), server))
		},
	}

	var botCmd = &cobra.Command{
		Use:   "bot",
		Short: "Start a sample auto-reply bot",
		Run: func(cmd *cobra.Command, args []string) {
			home, _ := os.UserHomeDir()
			dbPath := filepath.Join(home, ".trakand_reach", "reach.db")
			repo, _ := db.NewRepository(dbPath)
			manager, _ := engine.NewManager(repo)
			manager.Start()
			defer manager.Stop()

			session := &models.Session{
				ID: "bot-session",
				DeviceInfo: models.DeviceInfo{
					UserAgent:  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
					Width:      1280,
					Height:     720,
					PixelRatio: 1.0,
				},
				LastURL: "https://web.whatsapp.com",
			}

			inst, _ := manager.StartSession(session)

			go func() {
				for ev := range inst.Events {
					if ev.Type == "message_new" {
						data := ev.Data.(map[string]interface{})
						body := data["body"].(string)
						from := data["from"].(string)
						fmt.Printf("New message from %s: %s\n", from, body)
						if body == "hello" {
							manager.SendMessage(session.ID, from, "Hello! I am a Trakand Reach Go Bot.")
						}
					}
				}
			}()

			server := api.NewServer(manager)
			log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), server))
		},
	}

	setupCmd.Flags().IntVarP(&port, "port", "p", 3000, "Port to run on")

	rootCmd.AddCommand(runCmd, installCmd, setupCmd, uninstallCmd, whatsappCmd, botCmd)
	rootCmd.Execute()
}
