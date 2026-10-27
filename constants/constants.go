// Copyright (c) 2026 Михаил Попов
// ./maintenance_1C/constants/constants.go
package constants

const (
	Version         string = "1.2.21" //Исправлена ошибка - при неверных данных авторизации в базах 1С в secrets.yaml, программа "зависала" из-за того, что утилита ibcmd пыталась интерактивно запросить данные для авторизации.
	ScheduleDaily   string = "daily"
	ScheduleWeekly  string = "weekly"
	ScheduleMonthly string = "monthly"
	ValidPGSQLName  string = "PostgreSQL"
	ValidMSSQLName  string = "MSSQLServer"
	TmpTestdb       string = "tmptestdb"
)
