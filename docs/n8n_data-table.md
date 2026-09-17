## n8n data-table

Manage data tables, their rows and their columns

### Synopsis

Manage n8n data tables: the table itself, the rows in it, and the columns that
define it. Start with 'list' to find table IDs, then work on rows under the
'row' subgroup and on the schema under the 'column' subgroup.

Every command here requires API-key authentication; a bearer token or auth
cookie is refused before any request. Row values are typed: a value that parses
as JSON is sent as JSON (30, true, null, {"a":1}), anything else as text.
Deleting a table, clearing its rows, and deleting a column all destroy data and
require confirmation; filtered row updates and deletes support the server-side
--dry-run, which reports what would change without changing it.

```
n8n data-table [flags]
```

### Examples

```
  n8n data-table list
  n8n data-table create customers --column email:string --column age:number
  n8n data-table row list TABLE_ID --where status=active
  n8n data-table row insert TABLE_ID --set email=a@example.com --set age=30
  n8n data-table row delete TABLE_ID --where status=archived --dry-run
  n8n data-table column list TABLE_ID
```

### Options

```
  -h, --help   help for data-table
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n data-table column](n8n_data-table_column.md)	 - Manage the columns of a data table
* [n8n data-table create](n8n_data-table_create.md)	 - Create a data table
* [n8n data-table delete](n8n_data-table_delete.md)	 - Permanently delete a data table and every row in it
* [n8n data-table get](n8n_data-table_get.md)	 - Show one data table and its columns
* [n8n data-table list](n8n_data-table_list.md)	 - List data tables
* [n8n data-table row](n8n_data-table_row.md)	 - Read and change the rows of a data table
* [n8n data-table update](n8n_data-table_update.md)	 - Rename a data table

