package util

import (
	"encoding/json"
	"errors"
	"time"
)

var ValueError = errors.New("value error")

type Duration time.Duration

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}

	du, err := time.ParseDuration(v)
	if err != nil {
		return err
	}

	*d = Duration(du)
	return nil
}
