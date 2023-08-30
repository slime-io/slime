package bootstrap

import "reflect"

func Patch(target, patch *RegistryArgs) {
	innerPatchStruct(target, patch)
}

// innerPatchStruct patch the patch to the target in place, the input target and patch must be pointer to struct
func innerPatchStruct(target, patch interface{}) {
	tt, pt := reflect.TypeOf(target), reflect.TypeOf(patch)
	if tt.Kind() != reflect.Ptr || pt.Kind() != reflect.Ptr {
		return
	}
	tt, pt = tt.Elem(), pt.Elem()
	if tt.Kind() != reflect.Struct || pt.Kind() != reflect.Struct || tt != pt {
		return
	}

	tv, pv := reflect.ValueOf(target).Elem(), reflect.ValueOf(patch).Elem()

	for i := 0; i < pv.NumField(); i++ {
		if !tv.Field(i).CanSet() {
			continue
		}
		switch tv.Field(i).Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64, reflect.String, reflect.Bool:
			// todo: can't patch zero value
			if pv.Field(i).IsZero() {
				continue
			}
			tv.Field(i).Set(pv.Field(i))
		case reflect.Struct:
			innerPatchStruct(tv.Field(i).Addr().Interface(), pv.Field(i).Addr().Interface())
		case reflect.Ptr:
			if pv.Field(i).IsNil() {
				continue
			}
			if tv.Field(i).IsNil() {
				tv.Field(i).Set(reflect.New(tv.Field(i).Type().Elem()))
			}
			innerPatchStruct(tv.Field(i).Interface(), pv.Field(i).Interface())
		case reflect.Slice:
			if pv.Field(i).IsNil() {
				continue
			}
			if tv.Field(i).IsNil() {
				tv.Field(i).Set(reflect.MakeSlice(tv.Field(i).Type(), 0, pv.Field(i).Len()))
			}
			for j := 0; j < pv.Field(i).Len(); j++ {
				tv.Field(i).Set(reflect.Append(tv.Field(i), pv.Field(i).Index(j)))
			}
		case reflect.Map:
			if pv.Field(i).IsNil() {
				continue
			}
			if tv.Field(i).IsNil() {
				tv.Field(i).Set(reflect.MakeMap(tv.Field(i).Type()))
			}
			for _, k := range pv.Field(i).MapKeys() {
				// overwrite the value if the key already exists
				tv.Field(i).SetMapIndex(k, pv.Field(i).MapIndex(k))
			}
		default:
		}
	}
}
