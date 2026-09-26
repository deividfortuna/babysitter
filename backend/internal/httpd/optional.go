package httpd

import "encoding/json"

type Optional[T any] struct {
	Set   bool
	Value *T
}

func (o Optional[T]) IsZero() bool { return !o.Set }

func (o *Optional[T]) UnmarshalJSON(b []byte) error {
	o.Set, o.Value = true, nil
	if string(b) == "null" {
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

func (o Optional[T]) MarshalJSON() ([]byte, error) { return json.Marshal(o.Value) }
