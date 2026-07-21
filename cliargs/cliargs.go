package cliargs

import "flag"

// DEBUG экспортируется (доступна снаружи пакета)
var DEBUG bool

// ParseAll регистрирует флаговые переменные и парсит аргументы
func ParseAll() {
	flag.BoolVar(&DEBUG, "debug", false, "Включение режима отладки")

	// Парсим аргументы прямо внутри пакета
	flag.Parse()
}
