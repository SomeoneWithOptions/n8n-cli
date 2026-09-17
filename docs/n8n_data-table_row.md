## n8n data-table row

Read and change the rows of a data table

### Synopsis

Work with the contents of one data table: list rows, insert them, update or
upsert the ones matching a filter, delete the ones matching a filter, or clear
the table entirely.

Rows are selected with repeatable --where expressions: COLUMN=VALUE compares
with eq, and COLUMN=CONDITION=VALUE picks another condition (eq, neq, like, ilike, gt, gte, lt, lte).
Several --where expressions combine with and, or with or when --match or is
given; --filter takes the whole documented filter object as JSON instead.
Values are typed: 30, true and null are sent as JSON, other text as a string.
Update, upsert and delete accept --dry-run, which asks the server to report the
before and after rows without writing anything.

```
n8n data-table row [flags]
```

### Examples

```
  n8n data-table row list TABLE_ID --where status=active --sort-by createdAt:desc
  n8n data-table row insert TABLE_ID --set email=a@example.com --set age=30 --return all
  n8n data-table row update TABLE_ID --where status=pending --set status=done --dry-run
  n8n data-table row upsert TABLE_ID --where email=a@example.com --set name=Jane
  n8n data-table row delete TABLE_ID --where status=archived --yes
  n8n data-table row clear TABLE_ID --yes
```

### Options

```
  -h, --help   help for row
```

### SEE ALSO

* [n8n data-table](n8n_data-table.md)	 - Manage data tables, their rows and their columns
* [n8n data-table row clear](n8n_data-table_row_clear.md)	 - Permanently delete every row of a data table
* [n8n data-table row delete](n8n_data-table_row_delete.md)	 - Permanently delete the rows matching a filter
* [n8n data-table row insert](n8n_data-table_row_insert.md)	 - Insert rows into a data table
* [n8n data-table row list](n8n_data-table_row_list.md)	 - List the rows of a data table
* [n8n data-table row update](n8n_data-table_row_update.md)	 - Update every row matching a filter
* [n8n data-table row upsert](n8n_data-table_row_upsert.md)	 - Update the row matching a filter, or insert it

