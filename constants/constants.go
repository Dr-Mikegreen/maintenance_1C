// Copyright (c) 2026 Михаил Попов
// ./maintenance_1C/constants/constants.go
package constants

const (
	Version         string = "1.2.20" //Добавлена проверка выгрузки путем сопоставления с размером предыдущей копии. Исправлены обнаруженные ошибки
	ScheduleDaily   string = "daily"
	ScheduleWeekly  string = "weekly"
	ScheduleMonthly string = "monthly"
	ValidPGSQLName  string = "PostgreSQL"
	ValidMSSQLName  string = "MSSQLServer"
	TmpTestdb       string = "tmptestdb"
)
