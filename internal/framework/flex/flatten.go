// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package flex

import (
	"context"
	"fmt"
	"reflect"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-provider-aws/internal/errs/fwdiag"
)

// TODO
// TODO Return Diagnostics, not error.
// TODO Add a post-func to tidy up.
// TODO

// Flatten "flattens" an AWS SDK for Go v2 API data structure into
// a resource's "business logic" data structure, implemented using
// Terraform Plugin Framework data types.
// The API data structure's fields are walked and exported fields that
// have a corresponding field in the resource's data structure (and a
// suitable target data type) are copied.
func Flatten(ctx context.Context, apiObject, tfObject any) error {
	if err := walkStructFields(ctx, apiObject, tfObject, flattenVisitor{}); err != nil {
		return fmt.Errorf("Flatten[%T, %T]: %w", apiObject, tfObject, err)
	}

	return nil
}

type flattenVisitor struct{}

func (visitor flattenVisitor) visit(ctx context.Context, fieldName string, valFrom, valTo reflect.Value) error {
	vTo, ok := valTo.Interface().(attr.Value)
	if !ok {
		return fmt.Errorf("does not implement attr.Value: %s", valTo.Kind())
	}

	kFrom, tTo := valFrom.Kind(), vTo.Type(ctx)
	switch kFrom {
	// Primitive types.
	case reflect.Bool:
		diags := visitor.bool(ctx, valFrom, tTo, valTo)
		return fwdiag.DiagnosticsError(diags)

	case reflect.Float32, reflect.Float64:
		diags := visitor.float(ctx, valFrom, tTo, valTo)
		return fwdiag.DiagnosticsError(diags)

	case reflect.Int32, reflect.Int64:
		diags := visitor.int(ctx, valFrom, tTo, valTo)
		return fwdiag.DiagnosticsError(diags)

	case reflect.String:
		diags := visitor.string(ctx, valFrom, tTo, valTo)
		return fwdiag.DiagnosticsError(diags)

	// Pointer to primitive types.
	case reflect.Ptr:
		diags := visitor.pointer(ctx, valFrom, tTo, valTo)
		return fwdiag.DiagnosticsError(diags)

	// Slice of primitive types or pointer to primitive types.
	case reflect.Slice:
		diags := visitor.slice(ctx, valFrom, tTo, valTo)
		return fwdiag.DiagnosticsError(diags)

		// Map of simple types or pointer to simple types.
	case reflect.Map:
		switch tMapKey := valFrom.Type().Key(); tMapKey.Kind() {
		case reflect.String:
			vFrom := valFrom.Interface()
			switch tMapElem := valFrom.Type().Elem(); tMapElem.Kind() {
			case reflect.String:
				switch tTo := tTo.(type) {
				case basetypes.MapTypable:
					//
					// map[string]string -> types.Map(OfString).
					//
					if vFrom != nil {
						v, diags := tTo.ValueFromMap(ctx, FlattenFrameworkStringValueMap(ctx, vFrom.(map[string]string)))
						if err := fwdiag.DiagnosticsError(diags); err != nil {
							return err
						}
						valTo.Set(reflect.ValueOf(v))
					} else {
						valTo.Set(reflect.ValueOf(types.MapNull(types.StringType)))
					}
					return nil
				}

			case reflect.Ptr:
				switch tMapElem.Elem().Kind() {
				case reflect.String:
					switch tTo := tTo.(type) {
					case basetypes.MapTypable:
						//
						// map[string]*string -> types.Map(OfString).
						//
						if vFrom != nil {
							v, diags := tTo.ValueFromMap(ctx, FlattenFrameworkStringMap(ctx, vFrom.(map[string]*string)))
							if err := fwdiag.DiagnosticsError(diags); err != nil {
								return err
							}
							valTo.Set(reflect.ValueOf(v))
						} else {
							valTo.Set(reflect.ValueOf(types.MapNull(types.StringType)))
						}
						return nil
					}
				}
			}
		}
	}

	return fmt.Errorf("incompatible (%s): %s", kFrom, tTo)
}

// bool copies an AWS API bool value to a compatible Plugin Framework field.
func (visitor flattenVisitor) bool(ctx context.Context, vFrom reflect.Value, tTo attr.Type, vTo reflect.Value) diag.Diagnostics {
	var diags diag.Diagnostics

	switch tTo := tTo.(type) {
	case basetypes.BoolTypable:
		v, d := tTo.ValueFromBool(ctx, types.BoolValue(vFrom.Bool()))
		diags.Append(d...)
		if diags.HasError() {
			return diags
		}

		//
		// bool -> types.Bool.
		//
		vTo.Set(reflect.ValueOf(v))
		return diags
	}

	diags.Append(visitor.newIncompatibleTypesError(ctx, vFrom, tTo))

	return diags
}

// float copies an AWS API float value to a compatible Plugin Framework field.
func (visitor flattenVisitor) float(ctx context.Context, vFrom reflect.Value, tTo attr.Type, vTo reflect.Value) diag.Diagnostics {
	var diags diag.Diagnostics

	switch tTo := tTo.(type) {
	case basetypes.Float64Typable:
		v, d := tTo.ValueFromFloat64(ctx, types.Float64Value(vFrom.Float()))
		diags.Append(d...)
		if diags.HasError() {
			return diags
		}

		//
		// float32/float64 -> types.Float64.
		//
		vTo.Set(reflect.ValueOf(v))
		return diags
	}

	diags.Append(visitor.newIncompatibleTypesError(ctx, vFrom, tTo))

	return diags
}

// int copies an AWS API int value to a compatible Plugin Framework field.
func (visitor flattenVisitor) int(ctx context.Context, vFrom reflect.Value, tTo attr.Type, vTo reflect.Value) diag.Diagnostics {
	var diags diag.Diagnostics

	switch tTo := tTo.(type) {
	case basetypes.Int64Typable:
		v, d := tTo.ValueFromInt64(ctx, types.Int64Value(vFrom.Int()))
		diags.Append(d...)
		if diags.HasError() {
			return diags
		}

		//
		// int32/int64 -> types.Int64.
		//
		vTo.Set(reflect.ValueOf(v))
		return diags
	}

	diags.Append(visitor.newIncompatibleTypesError(ctx, vFrom, tTo))

	return diags
}

// string copies an AWS API string value to a compatible Plugin Framework field.
func (visitor flattenVisitor) string(ctx context.Context, vFrom reflect.Value, tTo attr.Type, vTo reflect.Value) diag.Diagnostics {
	var diags diag.Diagnostics

	switch tTo := tTo.(type) {
	case basetypes.StringTypable:
		v, d := tTo.ValueFromString(ctx, types.StringValue(vFrom.String()))
		diags.Append(d...)
		if diags.HasError() {
			return diags
		}

		//
		// string -> types.String.
		//
		vTo.Set(reflect.ValueOf(v))
		return diags
	}

	diags.Append(visitor.newIncompatibleTypesError(ctx, vFrom, tTo))

	return diags
}

// pointer copies an AWS API pointer value to a compatible Plugin Framework field.
func (visitor flattenVisitor) pointer(ctx context.Context, vFrom reflect.Value, tTo attr.Type, vTo reflect.Value) diag.Diagnostics {
	var diags diag.Diagnostics

	switch vElem := vFrom.Elem(); vFrom.Type().Elem().Kind() {
	case reflect.Bool:
		if vFrom.IsNil() {
			vTo.Set(reflect.ValueOf(types.BoolNull()))
			return diags
		}

		diags.Append(visitor.bool(ctx, vElem, tTo, vTo)...)
		return diags

	case reflect.Float32, reflect.Float64:
		if vFrom.IsNil() {
			vTo.Set(reflect.ValueOf(types.Float64Null()))
			return diags
		}

		diags.Append(visitor.float(ctx, vElem, tTo, vTo)...)
		return diags

	case reflect.Int32, reflect.Int64:
		if vFrom.IsNil() {
			vTo.Set(reflect.ValueOf(types.Int64Null()))
			return diags
		}

		diags.Append(visitor.int(ctx, vElem, tTo, vTo)...)
		return diags

	case reflect.String:
		if vFrom.IsNil() {
			vTo.Set(reflect.ValueOf(types.StringNull()))
			return diags
		}

		diags.Append(visitor.string(ctx, vElem, tTo, vTo)...)
		return diags

	case reflect.Struct:
		switch tTo.(type) {
		case basetypes.ListTypable:
			//
			// *struct -> types.List(OfObject).
			//
			return diags

		case basetypes.SetTypable:
			//
			// *struct -> types.Set(OfObject).
			//
			return diags
		}
	}

	diags.Append(visitor.newIncompatibleTypesError(ctx, vFrom, tTo))

	return diags
}

// slice copies an AWS API slice value to a compatible Plugin Framework field.
func (visitor flattenVisitor) slice(ctx context.Context, vFrom reflect.Value, tTo attr.Type, vTo reflect.Value) diag.Diagnostics {
	var diags diag.Diagnostics

	switch tSliceElem := vFrom.Type().Elem(); tSliceElem.Kind() {
	case reflect.String:
		switch tTo := tTo.(type) {
		case basetypes.ListTypable:
			//
			// []string -> types.List(OfString).
			//
			if vFrom.IsNil() {
				vTo.Set(reflect.ValueOf(types.ListNull(types.StringType)))
				return diags
			}

			v, d := tTo.ValueFromList(ctx, FlattenFrameworkStringValueList(ctx, vFrom.Interface().([]string)))
			diags.Append(d...)
			if diags.HasError() {
				return diags
			}

			vTo.Set(reflect.ValueOf(v))
			return diags

		case basetypes.SetTypable:
			//
			// []string -> types.Set(OfString).
			//
			if vFrom.IsNil() {
				vTo.Set(reflect.ValueOf(types.SetNull(types.StringType)))
				return diags
			}

			v, d := tTo.ValueFromSet(ctx, FlattenFrameworkStringValueSet(ctx, vFrom.Interface().([]string)))
			diags.Append(d...)
			if diags.HasError() {
				return diags
			}

			vTo.Set(reflect.ValueOf(v))
			return diags
		}

	case reflect.Ptr:
		switch tSliceElem.Elem().Kind() {
		case reflect.String:
			switch tTo := tTo.(type) {
			case basetypes.ListTypable:
				//
				// []*string -> types.List(OfString).
				//
				if vFrom.IsNil() {
					vTo.Set(reflect.ValueOf(types.ListNull(types.StringType)))
					return diags
				}

				v, d := tTo.ValueFromList(ctx, FlattenFrameworkStringList(ctx, vFrom.Interface().([]*string)))
				diags.Append(d...)
				if diags.HasError() {
					return diags
				}

				vTo.Set(reflect.ValueOf(v))
				return diags

			case basetypes.SetTypable:
				//
				// []string -> types.Set(OfString).
				//
				if vFrom.IsNil() {
					vTo.Set(reflect.ValueOf(types.SetNull(types.StringType)))
					return diags
				}

				v, d := tTo.ValueFromSet(ctx, FlattenFrameworkStringSet(ctx, vFrom.Interface().([]*string)))
				diags.Append(d...)
				if diags.HasError() {
					return diags
				}

				vTo.Set(reflect.ValueOf(v))
				return diags
			}
		}
	}

	diags.Append(visitor.newIncompatibleTypesError(ctx, vFrom, tTo))

	return diags
}

func (visitor flattenVisitor) newIncompatibleTypesError(ctx context.Context, vFrom reflect.Value, tTo attr.Type) diag.ErrorDiagnostic {
	return diag.NewErrorDiagnostic("Incompatible types", fmt.Sprintf("%s cannot be flattened to %s", vFrom.Kind(), tTo))
}
