package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/internal/client"
	"github.com/tako0614/terraform-provider-takoform/internal/indexedsql"
)

var (
	sqlDatabaseColumnAttrTypes = map[string]attr.Type{
		"name": types.StringType, "type": types.StringType, "nullable": types.BoolType,
	}
	sqlDatabaseIndexAttrTypes = map[string]attr.Type{
		"name":    types.StringType,
		"columns": types.ListType{ElemType: types.StringType},
		"unique":  types.BoolType,
	}
	sqlDatabaseTableAttrTypes = map[string]attr.Type{
		"name":        types.StringType,
		"columns":     types.ListType{ElemType: types.ObjectType{AttrTypes: sqlDatabaseColumnAttrTypes}},
		"primary_key": types.ListType{ElemType: types.StringType},
		"indexes":     types.ListType{ElemType: types.ObjectType{AttrTypes: sqlDatabaseIndexAttrTypes}},
	}
)

type sqlDatabaseColumnModel struct {
	Name     types.String `tfsdk:"name"`
	Type     types.String `tfsdk:"type"`
	Nullable types.Bool   `tfsdk:"nullable"`
}

type sqlDatabaseIndexModel struct {
	Name    types.String `tfsdk:"name"`
	Columns types.List   `tfsdk:"columns"`
	Unique  types.Bool   `tfsdk:"unique"`
}

type sqlDatabaseTableModel struct {
	Name       types.String `tfsdk:"name"`
	Columns    types.List   `tfsdk:"columns"`
	PrimaryKey types.List   `tfsdk:"primary_key"`
	Indexes    types.List   `tfsdk:"indexes"`
}

func sqlDatabaseTablesAttribute() schema.ListNestedAttribute {
	identifier := []validator.String{StringMatches(`^[A-Za-z][A-Za-z0-9_]{0,63}$`, "identifier must start with a letter and contain only letters, digits, or underscore")}
	return schema.ListNestedAttribute{
		Optional:    true,
		Description: "Immutable SQLDatabase@2.0.0 bounded table schema. Configuring tables selects data.indexed@1 and sends no engine or migrations path; changing tables replaces the Resource.",
		Validators:  []validator.List{ListSizeBetween(1, indexedsql.MaxTables)},
		PlanModifiers: []planmodifier.List{
			listplanmodifier.RequiresReplace(),
		},
		NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required: true, Description: "Portable table identifier.", Validators: identifier,
			},
			"columns": schema.ListNestedAttribute{
				Required: true, Description: "Closed scalar column definitions.",
				Validators: []validator.List{ListSizeBetween(1, indexedsql.MaxColumnsPerTable)},
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{
						Required: true, Description: "Portable column identifier.", Validators: identifier,
					},
					"type": schema.StringAttribute{
						Required: true, Description: "Portable scalar type: string, integer, number, or boolean.",
						Validators: []validator.String{StringOneOf("string", "integer", "number", "boolean")},
					},
					"nullable": schema.BoolAttribute{
						Optional: true, Computed: true, Default: booldefault.StaticBool(false),
						Description: "Whether the column accepts null. Primary-key and index columns must be non-null.",
					},
				}},
			},
			"primary_key": schema.ListAttribute{
				Required: true, ElementType: types.StringType,
				Description: "Ordered primary-key columns. Every entry must name a declared non-null string, integer, or boolean column.",
				Validators:  []validator.List{ListSizeBetween(1, indexedsql.MaxKeyColumns)},
			},
			"indexes": schema.ListNestedAttribute{
				Optional: true, Description: "Declared bounded indexes used by get_unique and page operations.",
				Validators: []validator.List{ListSizeBetween(0, indexedsql.MaxIndexesPerTable)},
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{
						Required: true, Description: "Portable index identifier.", Validators: identifier,
					},
					"columns": schema.ListAttribute{
						Required: true, ElementType: types.StringType,
						Description: "Ordered index columns. Every entry must name a declared non-null string, integer, or boolean column.",
						Validators:  []validator.List{ListSizeBetween(1, indexedsql.MaxKeyColumns)},
					},
					"unique": schema.BoolAttribute{
						Optional: true, Computed: true, Default: booldefault.StaticBool(false),
						Description: "Whether the declared index enforces uniqueness.",
					},
				}},
			},
		}},
	}
}

func sqlDatabaseUsesV2(tables types.List) bool {
	return !tables.IsNull() && !tables.IsUnknown()
}

func (r *serviceShapeResource) formForModel(model serviceShapeModel) (client.InstalledFormReference, bool) {
	if r.cfg.spec == specSQLDatabase {
		// An unknown tables value is neither historical 1.x nor confirmed 2.x.
		// Never let it select the historical Form by absence.
		if model.Tables.IsUnknown() {
			return client.InstalledFormReference{}, false
		}
		if sqlDatabaseUsesV2(model.Tables) {
			return providerCandidateFormVersion(client.KindSQLDatabase, indexedsql.DefinitionVersion)
		}
	}
	form, ok := r.data.forms[r.cfg.kind]
	return form, ok
}

func sqlDatabaseTablesToSpec(ctx context.Context, value types.List, diags *diag.Diagnostics) []any {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	var tables []sqlDatabaseTableModel
	diags.Append(value.ElementsAs(ctx, &tables, false)...)
	if diags.HasError() {
		return nil
	}
	out := make([]any, 0, len(tables))
	for tableIndex, table := range tables {
		if table.Name.IsNull() || table.Name.IsUnknown() {
			diags.AddAttributeError(path.Root("tables").AtListIndex(tableIndex).AtName("name"), "Invalid table", "table name must be known")
			return nil
		}
		var columns []sqlDatabaseColumnModel
		diags.Append(table.Columns.ElementsAs(ctx, &columns, false)...)
		if diags.HasError() {
			return nil
		}
		columnValues := make([]any, 0, len(columns))
		for _, column := range columns {
			columnValues = append(columnValues, map[string]any{
				"name": column.Name.ValueString(), "type": column.Type.ValueString(), "nullable": column.Nullable.ValueBool(),
			})
		}
		var primaryKey []string
		diags.Append(table.PrimaryKey.ElementsAs(ctx, &primaryKey, false)...)
		if diags.HasError() {
			return nil
		}
		primaryKeyValues := make([]any, 0, len(primaryKey))
		for _, column := range primaryKey {
			primaryKeyValues = append(primaryKeyValues, column)
		}
		tableValue := map[string]any{
			"name": table.Name.ValueString(), "columns": columnValues, "primaryKey": primaryKeyValues,
		}
		if !table.Indexes.IsNull() && !table.Indexes.IsUnknown() {
			var indexes []sqlDatabaseIndexModel
			diags.Append(table.Indexes.ElementsAs(ctx, &indexes, false)...)
			if diags.HasError() {
				return nil
			}
			indexValues := make([]any, 0, len(indexes))
			for _, index := range indexes {
				var columns []string
				diags.Append(index.Columns.ElementsAs(ctx, &columns, false)...)
				if diags.HasError() {
					return nil
				}
				columnNames := make([]any, 0, len(columns))
				for _, column := range columns {
					columnNames = append(columnNames, column)
				}
				indexValues = append(indexValues, map[string]any{
					"name": index.Name.ValueString(), "columns": columnNames, "unique": index.Unique.ValueBool(),
				})
			}
			tableValue["indexes"] = indexValues
		}
		out = append(out, tableValue)
	}
	return out
}

func sqlDatabaseTablesFromSpec(ctx context.Context, raw any) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	tables, ok := raw.([]any)
	if !ok {
		return types.ListNull(types.ObjectType{AttrTypes: sqlDatabaseTableAttrTypes}), diags
	}
	values := make([]attr.Value, 0, len(tables))
	for tableIndex, rawTable := range tables {
		table, ok := rawTable.(map[string]any)
		if !ok {
			diags.AddError("Invalid SQLDatabase table", fmt.Sprintf("tables[%d] is not an object", tableIndex))
			return types.ListNull(types.ObjectType{AttrTypes: sqlDatabaseTableAttrTypes}), diags
		}
		columns, d := sqlDatabaseColumnsFromSpec(table["columns"])
		diags.Append(d...)
		primaryKey, d := stringListValue(table["primaryKey"])
		diags.Append(d...)
		indexes, d := sqlDatabaseIndexesFromSpec(table["indexes"])
		diags.Append(d...)
		if diags.HasError() {
			return types.ListNull(types.ObjectType{AttrTypes: sqlDatabaseTableAttrTypes}), diags
		}
		value, d := types.ObjectValue(sqlDatabaseTableAttrTypes, map[string]attr.Value{
			"name": types.StringValue(stringFromAny(table["name"])), "columns": columns,
			"primary_key": primaryKey, "indexes": indexes,
		})
		diags.Append(d...)
		values = append(values, value)
	}
	result, d := types.ListValue(types.ObjectType{AttrTypes: sqlDatabaseTableAttrTypes}, values)
	diags.Append(d...)
	return result, diags
}

func sqlDatabaseColumnsFromSpec(raw any) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	columns, ok := raw.([]any)
	if !ok {
		diags.AddError("Invalid SQLDatabase columns", "columns is not an array")
		return types.ListNull(types.ObjectType{AttrTypes: sqlDatabaseColumnAttrTypes}), diags
	}
	values := make([]attr.Value, 0, len(columns))
	for _, rawColumn := range columns {
		column, ok := rawColumn.(map[string]any)
		if !ok {
			diags.AddError("Invalid SQLDatabase column", "column is not an object")
			return types.ListNull(types.ObjectType{AttrTypes: sqlDatabaseColumnAttrTypes}), diags
		}
		nullable, _ := column["nullable"].(bool)
		value, d := types.ObjectValue(sqlDatabaseColumnAttrTypes, map[string]attr.Value{
			"name": types.StringValue(stringFromAny(column["name"])), "type": types.StringValue(stringFromAny(column["type"])),
			"nullable": types.BoolValue(nullable),
		})
		diags.Append(d...)
		values = append(values, value)
	}
	result, d := types.ListValue(types.ObjectType{AttrTypes: sqlDatabaseColumnAttrTypes}, values)
	diags.Append(d...)
	return result, diags
}

func sqlDatabaseIndexesFromSpec(raw any) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	if raw == nil {
		return types.ListNull(types.ObjectType{AttrTypes: sqlDatabaseIndexAttrTypes}), diags
	}
	indexes, ok := raw.([]any)
	if !ok {
		diags.AddError("Invalid SQLDatabase indexes", "indexes is not an array")
		return types.ListNull(types.ObjectType{AttrTypes: sqlDatabaseIndexAttrTypes}), diags
	}
	values := make([]attr.Value, 0, len(indexes))
	for _, rawIndex := range indexes {
		index, ok := rawIndex.(map[string]any)
		if !ok {
			diags.AddError("Invalid SQLDatabase index", "index is not an object")
			return types.ListNull(types.ObjectType{AttrTypes: sqlDatabaseIndexAttrTypes}), diags
		}
		columns, d := stringListValue(index["columns"])
		diags.Append(d...)
		unique, _ := index["unique"].(bool)
		value, d := types.ObjectValue(sqlDatabaseIndexAttrTypes, map[string]attr.Value{
			"name": types.StringValue(stringFromAny(index["name"])), "columns": columns, "unique": types.BoolValue(unique),
		})
		diags.Append(d...)
		values = append(values, value)
	}
	result, d := types.ListValue(types.ObjectType{AttrTypes: sqlDatabaseIndexAttrTypes}, values)
	diags.Append(d...)
	return result, diags
}

func stringListValue(raw any) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	strings := toStringSlice(raw)
	values := make([]attr.Value, 0, len(strings))
	for _, value := range strings {
		values = append(values, types.StringValue(value))
	}
	result, d := types.ListValue(types.StringType, values)
	diags.Append(d...)
	return result, diags
}
