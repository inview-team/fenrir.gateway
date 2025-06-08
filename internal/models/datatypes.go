package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

type JSONBMap map[string]string

func (m JSONBMap) Value() (driver.Value, error) {
	if m == nil {
		return json.Marshal(make(map[string]string))
	}
	return json.Marshal(m)
}

func (m *JSONBMap) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(b, &m)
}
