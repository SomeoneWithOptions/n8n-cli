## n8n data-table create

Create a data table

### Synopsis

Create one data table with its columns. Pass --column once per column as
NAME:TYPE; at least one is required, and the column order is the order given.

Documented types are string, number, boolean, date and json, though instances
can reject json with a 400. Without --project-id the table is created in the
caller's personal project. The system columns id, createdAt and updatedAt are
added by n8n; do not declare them. Add more columns later with
'n8n data-table column create'. Requires dataTable:create.

```
n8n data-table create <name> [flags]
```

### Examples

```
  n8n data-table create customers --column email:string --column age:number
  n8n data-table create orders --column total:number --column paid:boolean --column placed:date
  n8n data-table create customers --column email:string --project-id PROJECT_ID --output json
```

### Options

```
      --column stringArray   column as NAME:TYPE, repeatable and required, e.g. email:string (types: string, number, boolean, date, json)
      --context string       saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help                 help for create
      --output string        output format: text or json (JSON is the created table) (default "text")
      --project-id string    project to create the table in (default: the caller's personal project)
      --url string           instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n data-table](n8n_data-table.md)	 - Manage data tables, their rows and their columns

