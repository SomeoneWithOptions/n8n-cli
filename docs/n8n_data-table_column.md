## n8n data-table column

Manage the columns of a data table

### Synopsis

Manage the schema of one data table: list its columns, add one, rename or
reorder one, or delete one.

Columns have a type fixed at creation (string, number, boolean, date)
and a zero-based position. The system columns id, createdAt and updatedAt are
managed by n8n and are not listed here. Deleting a column deletes the data in
it for every row.

```
n8n data-table column [flags]
```

### Examples

```
  n8n data-table column list TABLE_ID
  n8n data-table column create TABLE_ID status string
  n8n data-table column update TABLE_ID COLUMN_ID --name state --index 0
  n8n data-table column delete TABLE_ID COLUMN_ID --yes
```

### Options

```
  -h, --help   help for column
```

### SEE ALSO

* [n8n data-table](n8n_data-table.md)	 - Manage data tables, their rows and their columns
* [n8n data-table column create](n8n_data-table_column_create.md)	 - Add a column to a data table
* [n8n data-table column delete](n8n_data-table_column_delete.md)	 - Permanently delete a column and its data
* [n8n data-table column list](n8n_data-table_column_list.md)	 - List the columns of a data table
* [n8n data-table column update](n8n_data-table_column_update.md)	 - Rename or reorder a column

