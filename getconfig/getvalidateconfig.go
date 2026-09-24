// Copyright (c) 2026 Михаил Попов
// ./maintenance_1C/getconfig/getvalidateconfig.go
package getconfig

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	General          General          `yaml:"general"`
	ServerSettings   ServerSettings   `yaml:"server_settings"`
	ScheduleSettings ScheduleSettings `yaml:"schedule_settings"`
	Bases            []Base           `yaml:"bases"`
	UtilName         string
	MountPoints      map[string]string
}

type General struct {
	BackupsPath   string `yaml:"backups_path"`
	IbcmdPath     string `yaml:"ibcmd_path"`
	LogPath       string `yaml:"log_path"`
	StopServise1C *bool  `yaml:"1c_stop"`
	TimeToStart   string `yaml:"time_to_start"`
	SecretsFile   string `yaml:"secrets_file"`
	MountPath     string `yaml:"mount_path"`
	NetUser       string `yaml:"net_user"`
	NetPassword   string `yaml:"net_password"`
}

type ServerSettings struct {
	DBMS         string `yaml:"dbms"`
	Server       string `yaml:"srv"`
	Port         int    `yaml:"port"`
	DBMSUser     string `yaml:"dbms_user"`
	DBMSPassword string `yaml:"dbms_password"`
}

type ScheduleSettings struct {
	Daily   ScheduleRule `yaml:"daily"`
	Weekly  ScheduleRule `yaml:"weekly"`
	Monthly ScheduleRule `yaml:"monthly"`
}

type ScheduleRule struct {
	ScheduleBackupDir string   `yaml:"schedule_backup_dir"`
	ScheduleDays      []int    `yaml:"schedule_days"`
	DeleteOlder       int      `yaml:"delete_older"` //Сколькок минут хранить ежедневные копии
	MaintenanceTypes  []string `yaml:"maintenance_types"`
}

type Base struct {
	Mode            string   `yaml:"mode"`
	DBDir           string   `yaml:"dbdir"`
	DBName          string   `yaml:"dbname"`
	User            string   `yaml:"user"`
	Password        string   `yaml:"password"`
	ChkDB           bool     `yaml:"chkdb"`
	ValidateRestore bool     `yaml:"validate_restore"`
	Schedules       []string `yaml:"schedules"`
}

// Ошибки конфигурации.
var ErrInvalid_BackupsPath = errors.New("не указан каталог для сохранения копий")
var ErrInvalid_IbcmdPath = errors.New("не указан путь к утилите ibcmd")
var ErrInvalid_LogPath = errors.New("не указан каталог для сохранения логов")
var ErrInvalid_StopServise1C = errors.New("1c_stop должен быть \"true\" или \"false\"")
var ErrInvalid_TimeToStart = errors.New("время запуска должно быть в формате \"чч:мм\"")
var ErrInvalid_MountPath = errors.New("не указан каталог для монтирования сетевых файловых баз")
var ErrInvalid_NetUser = errors.New("не указан пользователь для доступа к сетевым ресурсам")
var ErrInvalid_DBMS = errors.New("тип СУБД может принимать значения \"PostgreSQL\" или \"MSSQL\" с учетом регистра или быть не заполненным")
var ErrInvalid_DBMSempty = errors.New("тип СУБД не заполнен, однако в секции bases есть базы с режимом \"dbms\"")
var ErrInvalid_DBMSfilled = errors.New("в секции bases нет баз с режимом \"dbms\", тип СУБД заполнять не нужно")
var ErrInvalid_Server = errors.New("адрес/имя сервера не может быть пустым")
var ErrInvalid_Port = errors.New("недопустимое значение порта")
var ErrInvalid_DBMSUser = errors.New("не указано имя пользователя СУБД")
var ErrInvalid_DBMSPassword = errors.New("не указан пароль пользователя СУБД")
var ErrInvalid_DeleteOlder = errors.New("недопустимое значение времени хранения копий")
var ErrInvalid_MaintenanceTypes = errors.New("maintenance_types может отсутствовать или быть пустым. Допустимые значения: \"STATISTICS\", \"INDEXES\", \"INTEGRITY\"; значения не должны повторяться")
var ErrInvalid_Mode = errors.New("mode должен быть \"file\" или \"dbms\" с учетом регистра")
var ErrInvalid_DBDir = errors.New("не указан каталог файловой базы 1С")
var ErrInvalid_DBName = errors.New("не указано имя базы данных")
var Err_Schedules = errors.New("база не включена ни в одно расписание")
var Invalid_Schedules = errors.New("неверно указаны расписания")

func LoadConfig(file string, now bool) (*Config, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("ошибка чиения файла: %w", err)
	}
	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("ошибка разбора yaml: %w", err)
	}
	err = getSecrets(&config, config.General.SecretsFile)
	if err != nil {
		return nil, err
	}
	err = config.ConfigValidate(now)
	if err != nil {
		return nil, err
	}
	err = config.EnvironmentValidate(now)
	if err != nil {
		return nil, err
	}
	err = PrepareEnvironment(&config)
	if err != nil {
		return &config, err
	}
	return &config, nil
}

func getSecrets(c *Config, file string) error {
	if file == "" {
		return fmt.Errorf("не заполнен secrets_file в config.yaml")
	}
	info, err := os.Stat(file)
	if err != nil {
		return fmt.Errorf("не удалось проверить файл %s: %w", file, err)
	} else if info.IsDir() {
		return fmt.Errorf("%s это каталог", file)
	}
	if runtime.GOOS != "Linux" {
		perm := info.Mode().Perm()
		if perm&0044 != 0 {
			return fmt.Errorf("файл %s доступен для чтения не только владельцу (права: %o), исправьте через chmod 600", file, perm)
		}
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("ошибка чтения файла: %w", err)
	}
	type baseSecret struct {
		User     string `yaml:"user"`
		Password string `yaml:"password"`
	}
	type secrets struct {
		Net struct {
			NetUser     string `yaml:"net_user"`
			NetPassword string `yaml:"net_password"`
		} `yaml:"net"`
		DBMS struct {
			DBMSUser     string `yaml:"dbms_user"`
			DBMSPassword string `yaml:"dbms_password"`
		} `yaml:"dbms"`
		Bases map[string]baseSecret `yaml:"bases"`
	}
	var s secrets
	err = yaml.Unmarshal(data, &s)
	if err != nil {
		return fmt.Errorf("ошибка разбора yaml: %w", err)
	}
	c.General.NetUser = s.Net.NetUser
	c.General.NetPassword = s.Net.NetPassword
	c.ServerSettings.DBMSUser = s.DBMS.DBMSUser
	c.ServerSettings.DBMSPassword = s.DBMS.DBMSPassword
	for i := range c.Bases {
		secret, ok := s.Bases[c.Bases[i].DBName]
		if ok {
			c.Bases[i].User = secret.User
			c.Bases[i].Password = secret.Password
		}
	}
	return nil
}

func (c *Config) ConfigValidate(now bool) error {
	var errs []error
	if c.General.BackupsPath == "" {
		errs = append(errs, ErrInvalid_BackupsPath)
	}
	if c.General.IbcmdPath == "" {
		errs = append(errs, ErrInvalid_IbcmdPath)
	}
	if c.General.LogPath == "" {
		errs = append(errs, ErrInvalid_LogPath)
	}
	if c.General.StopServise1C == nil {
		errs = append(errs, ErrInvalid_StopServise1C)
	}
	if !now {
		_, err := time.Parse("15:04", c.General.TimeToStart)
		if err != nil {
			errs = append(errs, ErrInvalid_TimeToStart)
		}
	}
	if c.ServerSettings.DBMS != "PostgreSQL" && c.ServerSettings.DBMS != "MSSQL" && c.ServerSettings.DBMS != "" {
		errs = append(errs, ErrInvalid_DBMS)
	}
	if c.ScheduleSettings.Daily.DeleteOlder < 0 {
		errs = append(errs, fmt.Errorf("%w для daily", ErrInvalid_DeleteOlder))
	}
	if c.ScheduleSettings.Weekly.DeleteOlder < 0 {
		errs = append(errs, fmt.Errorf("%w для weekly", ErrInvalid_DeleteOlder))
	}
	if c.ScheduleSettings.Monthly.DeleteOlder < 0 {
		errs = append(errs, fmt.Errorf("%w для monthly", ErrInvalid_DeleteOlder))
	}
	if err := maintenanceTypesCheck(c.ScheduleSettings.Daily.MaintenanceTypes); err != nil {
		errs = append(errs, fmt.Errorf("ошибка maintenance_types в секции daily:\n%w", err))
	}
	if err := maintenanceTypesCheck(c.ScheduleSettings.Weekly.MaintenanceTypes); err != nil {
		errs = append(errs, fmt.Errorf("ошибка maintenance_types в секции weekly:\n%w", err))
	}
	if err := maintenanceTypesCheck(c.ScheduleSettings.Monthly.MaintenanceTypes); err != nil {
		errs = append(errs, fmt.Errorf("ошибка maintenance_types в секции monthly:\n%w", err))
	}
	dbms := false
	netPath := false
	for i, base := range c.Bases {
		if base.Mode != "file" && base.Mode != "dbms" {
			errs = append(errs, fmt.Errorf("в секции bases в настройках базы под номером %d %w", i+1, ErrInvalid_Mode))
		}
		if base.Mode == "file" && base.DBDir == "" {
			errs = append(errs, fmt.Errorf("в секции bases в настройках базы под номером %d %w", i+1, ErrInvalid_DBDir))
		}
		if base.Mode == "dbms" && base.DBName == "" {
			errs = append(errs, fmt.Errorf("в секции bases в настройках базы под номером %d %w", i+1, ErrInvalid_DBName))
		}
		if !now {
			if len(base.Schedules) == 0 {
				errs = append(errs, fmt.Errorf("в секции bases в настройках базы под номером %d %w", i+1, Err_Schedules))
			}
			if err := schedulesCheck(base.Schedules); err != nil {
				errs = append(errs, fmt.Errorf("в секции bases в настройках базы под номером %d %w", i+1, err))
			}
		}
		if base.Mode == "dbms" {
			dbms = true
		}
		if strings.HasPrefix(base.DBDir, "//") {
			netPath = true
		}
	}
	// server_settings проверяется только при наличии баз с mode: "dbms"
	if dbms && c.ServerSettings.DBMS == "" {
		errs = append(errs, ErrInvalid_DBMSempty)
	}
	if dbms && c.ServerSettings.DBMS != "" {
		if c.ServerSettings.Server == "" {
			errs = append(errs, ErrInvalid_Server)
		}
		if c.ServerSettings.Port < 1 || c.ServerSettings.Port > 65535 {
			errs = append(errs, ErrInvalid_Port)
		}
		if c.ServerSettings.DBMSUser == "" {
			errs = append(errs, ErrInvalid_DBMSUser)
		}
		if c.ServerSettings.DBMSPassword == "" {
			errs = append(errs, ErrInvalid_DBMSPassword)
		}
	}
	if !dbms && c.ServerSettings.DBMS != "" {
		errs = append(errs, ErrInvalid_DBMSfilled)
	}
	// mount_path и NetUser проверяется только при наличии сетевых баз
	if netPath {
		if c.General.MountPath == "" {
			errs = append(errs, ErrInvalid_MountPath)
		}
		if c.General.NetUser == "" {
			errs = append(errs, ErrInvalid_NetUser)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func maintenanceTypesCheck(maintenanceTypes []string) error {
	allowed := map[string]struct{}{
		"STATISTICS": {},
		"INDEXES":    {},
		"INTEGRITY":  {},
	}
	seen := make(map[string]struct{})
	for _, value := range maintenanceTypes {
		if _, ok := allowed[value]; !ok {
			return ErrInvalid_MaintenanceTypes
		}
		if _, ok := seen[value]; ok {
			return ErrInvalid_MaintenanceTypes
		}
		seen[value] = struct{}{}
	}
	return nil
}

func schedulesCheck(schedules []string) error {
	allowed := map[string]struct{}{
		"daily":   {},
		"weekly":  {},
		"monthly": {},
	}
	seen := make(map[string]struct{})
	for _, value := range schedules {
		if _, ok := allowed[value]; !ok {
			return Invalid_Schedules
		}
		if _, ok := seen[value]; ok {
			return Invalid_Schedules
		}
		seen[value] = struct{}{}
	}
	return nil
}

// Ошибки среды
var ErrEnv_ServerPort = errors.New("не удалось подключиться к серверу. Проверьте имя или адрес и порт сервера")

func (c *Config) EnvironmentValidate(now bool) error {
	var errs []error
	// Проверим существует и доступен ли каталог для сохранения копий
	err := checkPath(c.General.BackupsPath)
	if err != nil {
		errs = append(errs, fmt.Errorf("для сохранения копий %w", err))
	}
	// Проверим утилиту ibcmd по указанному пути
	switch runtime.GOOS {
	case "linux":
		name := "ibcmd"
		targetFile := filepath.Join(c.General.IbcmdPath, name)
		file, err := os.Open(targetFile)
		if err != nil {
			errs = append(errs, fmt.Errorf("не удалось открыть для чтения утилиту ibcmd:\n%w", err))
		} else {
			defer file.Close()
		}
		c.UtilName = name
	case "windows":
		name := "ibcmd.exe"
		targetFile := filepath.Join(c.General.IbcmdPath, name)
		file, err := os.Open(targetFile)
		if err != nil {
			errs = append(errs, fmt.Errorf("не удалось открыть для чтения утилиту ibcmd:\n%w", err))
		} else {
			defer file.Close()
		}
		c.UtilName = name
	default:
		errs = append(errs, fmt.Errorf("неподдерживаемая операционная система: %s", runtime.GOOS))
	}
	// Проверим существует и доступен ли каталог для сохранения логов
	err = checkPath(c.General.LogPath)
	if err != nil {
		errs = append(errs, fmt.Errorf("для сохранения логов %w", err))
	}
	// Проверим доступна ли СУБД
	if c.ServerSettings.DBMS != "" {
		conn, err := net.DialTimeout(
			"tcp",
			net.JoinHostPort(c.ServerSettings.Server, strconv.Itoa(c.ServerSettings.Port)),
			5*time.Second,
		)
		if err != nil {
			errs = append(errs, ErrEnv_ServerPort)
		} else {
			conn.Close()
		}
	}
	// Проверим существуют и доступны ли каталоги с файловыми базами
	for i, base := range c.Bases {
		if base.Mode == "file" && !strings.HasPrefix(base.DBDir, "//") {
			err = checkPath(base.DBDir)
			if err != nil {
				errs = append(errs, fmt.Errorf("в секции bases в настройках базы под номером %d %w", i+1, err))
			}
		}
	}
	// Проверим существуют и доступны ли каталоги для сохранения копий соответственно режимам
	// if !now {
	// 	if c.ScheduleSettings.Daily.ScheduleBackupDir
	// }

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func PrepareEnvironment(c *Config) error {
	var errs []error

	// Выясним есть ли в списке баз сетевые Linux пути
	netPath := false
	for _, base := range c.Bases {
		if base.Mode == "file" && strings.HasPrefix(base.DBDir, "//") {
			netPath = true
		}
	}
	if netPath {
		file, err := os.CreateTemp("", "cifs-credentials-*")
		if err != nil {
			return fmt.Errorf("не удалось создать временный файл:\n%w", err)
		}
		if err := file.Chmod(0600); err != nil {
			file.Close()
			if err := os.Remove(file.Name()); err != nil {
				errs = append(errs, fmt.Errorf("не удалось удалить временный файл %s: %v\n", file.Name(), err))
			}
			errs = append(errs, fmt.Errorf("не удалось установить права 0600 на временный файл %s:\n%w", file.Name(), err))
			return errors.Join(errs...)
		}
		_, err = file.WriteString("username=" + c.General.NetUser + "\n" + "password=" + c.General.NetPassword + "\n")
		if err != nil {
			file.Close()
			if err := os.Remove(file.Name()); err != nil {
				errs = append(errs, fmt.Errorf("не удалось удалить временный файл %s: %v\n", file.Name(), err))
			}
			errs = append(errs, fmt.Errorf("не удалось записать данные во временный файл %s:\n%w", file.Name(), err))
			return errors.Join(errs...)
		}
		if err := file.Close(); err != nil {
			if err := os.Remove(file.Name()); err != nil {
				errs = append(errs, fmt.Errorf("не удалось удалить временный файл %s: %v\n", file.Name(), err))
			}
			errs = append(errs, fmt.Errorf("не удалось закрыть временный файл %s:\n%w", file.Name(), err))
			return errors.Join(errs...)
		}
		c.MountPoints = make(map[string]string)
		_, err = exec.LookPath("mount.cifs")
		if err != nil {
			if err := os.Remove(file.Name()); err != nil {
				errs = append(errs, fmt.Errorf("не удалось удалить временный файл %s: %v\n", file.Name(), err))
			}
			errs = append(errs, fmt.Errorf("mount.cifs не найден: %w, проерьте установлена ли программа", err))
			return errors.Join(errs...)
		}
		for i, base := range c.Bases {
			if base.Mode == "file" && strings.HasPrefix(base.DBDir, "//") {
				mountDir := filepath.Join(c.General.MountPath, base.DBName)
				err = os.Mkdir(mountDir, 0755)
				if err != nil {
					errs = append(errs, fmt.Errorf("не удалось создать каталог для монтирования базы %s, %w", base.DBName, err))
					continue
				}
				cmd := exec.Command("mount.cifs", base.DBDir, mountDir, "-o", "credentials="+file.Name())
				output, err := cmd.CombinedOutput()
				if err != nil {
					errs = append(errs, fmt.Errorf("не удалось смонтировать %s: %w: %s", base.DBDir, err, strings.TrimSpace(string(output))))
					continue
				}
				err = checkPath(mountDir)
				if err != nil {
					errs = append(errs, fmt.Errorf("в секции bases в настройках базы под номером в точке монтирования %d %w", i+1, err))
					continue
				}
				c.MountPoints[base.DBName] = mountDir
			}
		}
		if err := os.Remove(file.Name()); err != nil {
			errs = append(errs, fmt.Errorf("не удалось удалить временный файл %s: %v\n", file.Name(), err))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func checkPath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("не удалось получить каталог:\n%w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("%s это не каталог", path)
	}
	testFile := filepath.Join(path, "test.txt")
	file, err := os.Create(testFile)
	if err != nil {
		return fmt.Errorf("не удалось создать тестовый файл в %s:\n%w", path, err)
	} else {
		if err := file.Close(); err != nil {
			os.Remove(testFile)
			return fmt.Errorf("не удалось закрыть тестовый файл:\n%w", err)
		}
		if err := os.Remove(testFile); err != nil {
			return fmt.Errorf("не удалось удалить тестовый файл:\n%w", err)
		}
	}
	return nil
}
