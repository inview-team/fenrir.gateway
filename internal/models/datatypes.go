package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

// JSONBMap реализует интерфейсы sql.Scanner и driver.Valuer
// для корректной сериализации/десериализации map[string]string в/из JSONB.
type JSONBMap map[string]string

// Value преобразует нашу карту в JSON-байт-слайс для сохранения в БД.
func (m JSONBMap) Value() (driver.Value, error) {
	if m == nil {
		return json.Marshal(make(map[string]string))
	}
	return json.Marshal(m)
}

// Scan преобразует JSON-байт-слайс из БД обратно в нашу карту.
func (m *JSONBMap) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(b, &m)
}
