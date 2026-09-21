// Copyright (c) 2026 Михаил Попов

package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maintenance1c/getconfig"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const version string = "1.0.9"

var now bool = false

type Command struct {
	Name        string
	Description string
}

var commands = []Command{
	{"--help", "Выводит эту справку"},
	{"--version", "Показать версию программы"},
	{"run", "Запускает программу для работы по расписанию"},
	{"run --now", "Выполняет заданные действия сразу игнорируя расписание"},
	{"install", "Устанвливает программу как службу и добавляет в автозапуск. (Функуия в разработке)"},
	{"uninstall", "Останавливает и удаляет установленную службу. (Функуия в разработке)"},
	{"status", "Показывает сосояние службы. (Функуия в разработке)"},
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("недостаточно аргументов для запуска")
		return
	}
	if len(os.Args) > 3 {
		fmt.Println("слишком много аргументов")
		return
	}
	if len(os.Args) > 2 {
		s := os.Args[1] + " " + os.Args[2]
		if s != commands[3].Name {
			fmt.Println("неверные аргументы")
			return
		}
		now = true
		config, err := getconfig.LoadConfig("config.yaml", now)
		if err != nil {
			fmt.Println(err)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		result, err := startBackup(config, ctx)
		if err != nil {
			fmt.Println(err)
			fmt.Println(string(result))
			return
		}
		fmt.Println(string(result))
		return
	}
	switch os.Args[1] {
	case commands[0].Name:
		for _, str := range commands {
			fmt.Printf("%s 	- %s\n", str.Name, str.Description)
		}
		return
	case commands[1].Name:
		fmt.Println(version)
		return
	case commands[2].Name:
		fmt.Printf("Здесь скоро что-то будет %s 	- %s\n", commands[2].Name, commands[2].Description)
		return
	case commands[4].Name:
		fmt.Printf("Здесь скоро что-то будет %s 	- %s\n", commands[4].Name, commands[4].Description)
		return
	case commands[5].Name:
		fmt.Printf("Здесь скоро что-то будет %s 	- %s\n", commands[5].Name, commands[5].Description)
		return
	case commands[6].Name:
		fmt.Printf("Здесь скоро что-то будет %s 	- %s\n", commands[6].Name, commands[6].Description)
		return
	default:
		fmt.Println("неверный аргумент")
		return
	}
}

func startBackup(config *getconfig.Config, ctx context.Context) ([]byte, error) {
	var output []byte
	var errs []error

	ibcmdPath := filepath.Join(config.General.IbcmdPath, config.UlilName)
	for _, base := range config.Bases {
		fmt.Printf("Сейчас работаем с базой %s\n", base.DBName)
		fmt.Println(base.DBDir)
		timestamp := time.Now().Format("2006-01-02_15-04-05")
		backupPathName := filepath.Join(config.General.BackupsPath, base.DBName+"_"+timestamp)
		if base.Mode == "dbms" {
			ibcmdArgs := []string{
				"infobase",
				"dump",
				"--dbms=" + config.ServerSettings.DBMS,
				"--db-server=" + config.ServerSettings.Server + " port=" + strconv.Itoa(config.ServerSettings.Port),
				"--db-user=" + config.ServerSettings.DBMSUser,
				"--db-pwd=" + config.ServerSettings.DBMSPassword,
				"--db-name=" + base.DBName,
			}
			if base.User != "" {
				ibcmdArgs = append(ibcmdArgs, "--user="+base.User)
			}
			if base.Password != "" {
				ibcmdArgs = append(ibcmdArgs, "--password="+base.Password)
			}
			ibcmdArgs = append(ibcmdArgs, backupPathName)
			output, err := runIbcmd(ctx, ibcmdPath, ibcmdArgs)
			if err != nil {
				errs = append(errs, err)
				output = append(output, output...)
			}
			output = append(output, output...)
		} else {
			ibcmdArgs := []string{
				"infobase",
				"dump",
			}
			if strings.HasPrefix(base.DBDir, "//") {
				ibcmdArgs = append(ibcmdArgs, "--db-path="+getconfig.MountPoints[base.DBName])
			} else {
				dbPath := filepath.Join(base.DBDir)
				ibcmdArgs = append(ibcmdArgs, "--db-path="+dbPath)

			}
			if base.User != "" {
				ibcmdArgs = append(ibcmdArgs, "--user="+base.User)
			}
			if base.Password != "" {
				ibcmdArgs = append(ibcmdArgs, "--password="+base.Password)
			}
			ibcmdArgs = append(ibcmdArgs, backupPathName)
			output, err := runIbcmd(ctx, ibcmdPath, ibcmdArgs)
			if err != nil {
				errs = append(errs, err)
				output = append(output, output...)
			}
			output = append(output, output...)
		}
		if strings.HasPrefix(base.DBDir, "//") {
			cmd := exec.Command("umount", getconfig.MountPoints[base.DBName])
			output, err := cmd.CombinedOutput()
			if err != nil {
				fmt.Printf("не удалось отмонтировать %s: %s: %s\n", getconfig.MountPoints[base.DBName], err, strings.TrimSpace(string(output)))
			}
			err = os.Remove(getconfig.MountPoints[base.DBName])
			if err != nil {
				fmt.Printf("не удалось удалить каталог %s\n", getconfig.MountPoints[base.DBName])
			}
		}
	}

	if len(errs) > 0 {
		return output, errors.Join(errs...)
	} else if len(output) > 0 {
		return output, nil
	}
	return output, nil
}

func runIbcmd(ctx context.Context, ibcmdPath string, ibcmdArgs []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, ibcmdPath, ibcmdArgs...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			fmt.Println(string(exitErr.Stderr))
			return output, exitErr
		} else if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
			return output, pathErr
		} else {
			fmt.Println(err)
			return output, err
		}
	}
	fmt.Println(string(output))
	return output, nil
}
