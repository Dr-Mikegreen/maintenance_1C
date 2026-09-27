// Copyright (c) 2026 Михаил Попов

package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maintenance1c/constants"
	"maintenance1c/getconfig"
	"maintenance1c/logs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Command struct {
	Name        string
	Description string
}

var commands = []Command{
	{"--help", "Выводит эту справку"},
	{"--version", "Показать версию программы"},
	{"run", "Запускает программу для работы по расписанию"},
	{"run --now", "Выполняет заданные действия сразу игнорируя расписание"},
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
		os.Args[1] = s
	}
	var now bool = false
	var config *getconfig.Config
	if os.Args[1] == commands[3].Name {
		now = true
	}
	if os.Args[1] == commands[2].Name || os.Args[1] == commands[3].Name {
		var err error
		config, err = getconfig.LoadConfig("config.yaml", now)
		if config == nil && err != nil {
			fmt.Println(err)
			return
		} else if err != nil {
			lost := 0
			for _, base := range config.Bases {
				if strings.HasPrefix(base.DBDir, "//") && len(config.MountPoints[base.DBName]) == 0 {
					lost++
				}
			}
			if lost == len(config.Bases) {
				fmt.Println(err)
				return
			} else {
				fmt.Println(err)
			}
		}
		logFileDate := time.Now().Format("2006-01-02")
		logFile := filepath.Join(config.General.LogPath, logFileDate+".log")
		file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Println("не удалось открыть лог-файл:", err)
			os.Exit(1)
		}
		defer file.Close()
		logs.Logger = logs.Startlog(file)
		logs.Logger.Info("Программа запущена", "Версия", constants.Version)
		fmt.Printf("Программа запущена. Версия %s\n", constants.Version)
	}
	switch os.Args[1] {
	case commands[0].Name:
		for _, str := range commands {
			fmt.Printf("%s 	- %s\n", str.Name, str.Description)
		}
		return
	case commands[1].Name:
		fmt.Println(constants.Version)
		return
	case commands[2].Name:
		logs.Logger.Info("Работа по расписанию")
		if len(config.ScheduleSettings) == 0 {
			logs.Logger.Error("Не включено ни одно расписание в schedule_settings")
			fmt.Println("Не включено ни одно расписание в schedule_settings")
			return
		}
		t := time.Now()
		mode := curentMode(config, t)
		if mode == "" {
			logs.Logger.Warn("В настройках расписаний на сегодня действий не предусмотрено")
			fmt.Println("На сегодня нет работы")
			rel, err := ReleaseEnvironment(config)
			if err != nil {
				logs.Logger.Warn("Не удалось очистить окружение", "Предупреждение", err.Error()+" "+string(rel))
				fmt.Printf("Не удалось очистить окружение: %s\n", err)
				fmt.Println(string(rel))
				return
			}
			logs.Logger.Info("Очищено окружение")
			fmt.Println(string(rel))
			return
		}
		logs.Logger.Info("На данный момент определено", "Расписание", mode)
		fmt.Printf("Cегодня работаем по расписанию %s\n", mode)
		//Удалим старые файлы выгрузки
		if config.ScheduleSettings[mode].DeleteOlder > 0 {
			curentBackupPath := filepath.Join(config.General.BackupsPath, config.ScheduleSettings[mode].ScheduleBackupDir)
			err := cleanupOldFiles(curentBackupPath, time.Duration(config.ScheduleSettings[mode].DeleteOlder)*time.Hour)
			if err != nil {
				logs.Logger.Warn("Не удалось удалить устаревшие копии", "Предупреждение", curentBackupPath+" "+err.Error())
				fmt.Println(err)
			}
		}
		//Выберем базы, которые включены в это расписание и выгрузим
		var selectedDBs []getconfig.Base
		for _, base := range config.Bases {
			for _, schedule := range base.Schedules {
				if schedule == mode {
					selectedDBs = append(selectedDBs, base)
				}
			}
		}
		ibcmdPath := filepath.Join(config.General.IbcmdPath, config.UtilName)
		for _, base := range selectedDBs {
			logs.Logger.Info("Начало работы с", "База", base.DBName)
			fmt.Printf("Начало работы с базой: %s\n", base.DBName)
			timestamp := time.Now().Format("2006-01-02_15-04-05")
			backupPathName := filepath.Join(config.General.BackupsPath, config.ScheduleSettings[mode].ScheduleBackupDir, base.DBName+"_"+timestamp)
			result, err := backupBase(config, ibcmdPath, backupPathName, base)
			if err != nil {
				logs.Logger.Warn("Не удалось выгрузить базу", "Предупреждение", err.Error()+" "+string(result))
				fmt.Printf("Не удалось выгрузить базу: %s\n", err)
				fmt.Println(string(result))
				continue
			}
			logs.Logger.Info("Выгружена база", "база", base.DBName, "результат", string(result))
			fmt.Println(string(result))
		}
		rel, err := ReleaseEnvironment(config)
		if err != nil {
			logs.Logger.Warn("Не удалось очистить окружение", "Предупреждение", err.Error()+" "+string(rel))
			fmt.Printf("Не удалось очистить окружение: %s\n", err)
			fmt.Println(string(rel))
			return
		}
		logs.Logger.Info("Очищено окружение")
		fmt.Println(string(rel))
		return
	case commands[3].Name:
		logs.Logger.Info("Запуск с ключом --now. Игнорируем расписание")
		ibcmdPath := filepath.Join(config.General.IbcmdPath, config.UtilName)
		for _, base := range config.Bases {
			logs.Logger.Info("Начало работы с", "База", base.DBName)
			fmt.Printf("Начало работы с базой: %s\n", base.DBName)
			timestamp := time.Now().Format("2006-01-02_15-04-05")
			backupPathName := filepath.Join(config.General.BackupsPath, base.DBName+"_"+timestamp)
			result, err := backupBase(config, ibcmdPath, backupPathName, base)
			if err != nil {
				logs.Logger.Warn("Не удалось выгрузить базу", "Предупреждение", err.Error()+" "+string(result))
				fmt.Printf("Не удалось выгрузить базу: %s\n", err)
				fmt.Println(string(result))
				continue
			}
			logs.Logger.Info("Выгружена база", "база", base.DBName, "результат", string(result))
			fmt.Println(string(result))
		}
		rel, err := ReleaseEnvironment(config)
		if err != nil {
			logs.Logger.Warn("Не удалось очистить окружение", "Предупреждение", err.Error()+" "+string(rel))
			fmt.Printf("Не удалось очистить окружение: %s\n", err)
			fmt.Println(string(rel))
			return
		}
		logs.Logger.Info("Очищено окружение")
		fmt.Println(string(rel))
		return
	default:
		fmt.Println("неверный аргумент")
		return
	}
}

func curentMode(c *getconfig.Config, t time.Time) string {
	if settings, ok := c.ScheduleSettings[constants.ScheduleMonthly]; ok {
		isLastMonthDay := t.AddDate(0, 0, 1).Month() != t.Month()
		for _, setDay := range settings.ScheduleDays {
			if isLastMonthDay && setDay >= t.Day() {
				return constants.ScheduleMonthly
			} else if setDay == t.Day() {
				return constants.ScheduleMonthly
			}
		}
	}
	if settings, ok := c.ScheduleSettings[constants.ScheduleWeekly]; ok {
		for _, setDay := range settings.ScheduleDays {
			if setDay == int(t.Weekday()) {
				return constants.ScheduleWeekly
			}
		}
	}
	if settings, ok := c.ScheduleSettings[constants.ScheduleDaily]; ok {
		for _, setDay := range settings.ScheduleDays {
			if setDay == int(t.Weekday()) {
				return constants.ScheduleDaily
			}
		}
	}
	return ""
}

func cleanupOldFiles(dir string, maxAge time.Duration) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("чтение каталога %s: %w", dir, err)
	}
	var errs []error
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			errs = append(errs, fmt.Errorf("получение сведений о файле %s: %w", entry.Name(), err))
			continue
		}
		if time.Since(info.ModTime()) > maxAge {
			path := filepath.Join(dir, entry.Name())
			if err := os.Remove(path); err != nil {
				errs = append(errs, fmt.Errorf("удаление файла %s: %w", path, err))
				continue
			}
			logs.Logger.Info("Удалён", "файл:", path)
			fmt.Println("Удалён:", path)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("ошибки при очистке: %w", errors.Join(errs...))
	}
	return nil
}

func backupBase(c *getconfig.Config, ibcmdPath, backupPathName string, base getconfig.Base) ([]byte, error) {
	var ibcmdArgs []string
	var output []byte
	var errs []error
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.Advansed.CtxTimeout)*time.Minute)
	if base.Mode == "dbms" {
		ibcmdArgs = []string{
			"infobase",
			"dump",
			"--dbms=" + c.ServerSettings.DBMS,
			"--db-server=" + c.ServerSettings.Server + " port=" + strconv.Itoa(c.ServerSettings.Port),
			"--db-user=" + c.ServerSettings.DBMSUser,
			"--db-pwd=" + c.ServerSettings.DBMSPassword,
			"--db-name=" + base.DBName,
		}
		if base.User != "" {
			ibcmdArgs = append(ibcmdArgs, "--user="+base.User)
		}
		if base.Password != "" {
			ibcmdArgs = append(ibcmdArgs, "--password="+base.Password)
		}
		ibcmdArgs = append(ibcmdArgs, backupPathName)
	} else {
		ibcmdArgs = []string{
			"infobase",
			"dump",
		}
		if strings.HasPrefix(base.DBDir, "//") {
			if path, ok := c.MountPoints[base.DBName]; ok == false {
				errs = append(errs, fmt.Errorf("база %s пропущена — сетевой каталог не был смонтирован", base.DBName))
				cancel()
				return output, errors.Join(errs...)
			} else {
				ibcmdArgs = append(ibcmdArgs, "--db-path="+path)
			}
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
	}
	cmdOutput, err := runIbcmd(ctx, ibcmdPath, ibcmdArgs)
	if err != nil {
		errs = append(errs, err)
	}
	output = append(output, cmdOutput...)
	cancel()
	if len(errs) > 0 {
		return output, errors.Join(errs...)
	}
	return output, nil
}

func ReleaseEnvironment(c *getconfig.Config) ([]byte, error) {
	var errs []error
	var output []byte
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.Advansed.CtxTimeout)*time.Minute)
	for _, base := range c.Bases {
		if strings.HasPrefix(base.DBDir, "//") {
			cmd := exec.CommandContext(ctx, "umount", c.MountPoints[base.DBName])
			cmdOutput, err := cmd.CombinedOutput()
			if err != nil {
				errs = append(errs, fmt.Errorf("не удалось отмонтировать %s: %s: %s\n", c.MountPoints[base.DBName], err, strings.TrimSpace(string(cmdOutput))))
			}
			err = os.Remove(c.MountPoints[base.DBName])
			if err != nil {
				errs = append(errs, fmt.Errorf("не удалось удалить каталог %s\n%w", c.MountPoints[base.DBName], err))
			}
			output = append(output, cmdOutput...)
		}
	}
	cancel()
	if len(errs) > 0 {
		return output, errors.Join(errs...)
	}
	return output, nil
}

func runIbcmd(ctx context.Context, ibcmdPath string, ibcmdArgs []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, ibcmdPath, ibcmdArgs...)
	output, err := cmd.Output()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			switch {
			case errors.Is(ctxErr, context.DeadlineExceeded):
				return output, fmt.Errorf("команда не уложилась в таймаут: %w", ctxErr)
			case errors.Is(ctxErr, context.Canceled):
				return output, fmt.Errorf("выполнение команды отменено: %w", ctxErr)
			default:
				return output, ctxErr
			}
		}
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			return output, fmt.Errorf("ibcmd завершился с кодом %d: %s: %w", exitErr.ExitCode(), exitErr.Stderr, err)
		} else if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
			return output, fmt.Errorf("не удалось выполнить операцию %s %s:\n%w", pathErr.Op, pathErr.Path, pathErr.Err)
		} else {
			return output, err
		}
	}
	return output, nil
}
