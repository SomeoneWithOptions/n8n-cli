# n8n command reference

Generated from the command help. Do not edit these files by hand:
edit the command's Short, Long, Example or flag usage and run `make docs`.

| Command | Description |
|---|---|
| [`n8n audit generate`](n8n_audit_generate.md) | Generate a security audit of the instance |
| [`n8n audit`](n8n_audit.md) | Generate security audits of an n8n instance |
| [`n8n auth login`](n8n_auth_login.md) | Store a credential for an n8n instance |
| [`n8n auth logout`](n8n_auth_logout.md) | Delete the stored credential for a context |
| [`n8n auth status`](n8n_auth_status.md) | Show the active context, instance and credential state |
| [`n8n auth`](n8n_auth.md) | Log in to an n8n instance and inspect stored credentials |
| [`n8n community-package install`](n8n_community-package_install.md) | Install a community package |
| [`n8n community-package list`](n8n_community-package_list.md) | List installed community packages |
| [`n8n community-package uninstall`](n8n_community-package_uninstall.md) | Uninstall a community package |
| [`n8n community-package update`](n8n_community-package_update.md) | Update an installed community package |
| [`n8n community-package`](n8n_community-package.md) | Manage community node packages |
| [`n8n completion`](n8n_completion.md) | Generate a shell completion script |
| [`n8n config context delete`](n8n_config_context_delete.md) | Delete a context and its stored credential |
| [`n8n config context list`](n8n_config_context_list.md) | List saved contexts |
| [`n8n config context use`](n8n_config_context_use.md) | Select the context used by subsequent commands |
| [`n8n config context`](n8n_config_context.md) | Manage saved instance contexts |
| [`n8n config`](n8n_config.md) | Inspect and edit stored CLI configuration |
| [`n8n credential create`](n8n_credential_create.md) | Create a credential from secret JSON |
| [`n8n credential delete`](n8n_credential_delete.md) | Permanently delete a credential |
| [`n8n credential get`](n8n_credential_get.md) | Get credential metadata |
| [`n8n credential list`](n8n_credential_list.md) | List credential metadata |
| [`n8n credential schema`](n8n_credential_schema.md) | Show credential data JSON Schema |
| [`n8n credential test`](n8n_credential_test.md) | Test stored credential data |
| [`n8n credential transfer`](n8n_credential_transfer.md) | Transfer a credential to another project |
| [`n8n credential update`](n8n_credential_update.md) | Update a credential from secret JSON |
| [`n8n credential`](n8n_credential.md) | Manage node credentials without exposing secrets |
| [`n8n data-table column create`](n8n_data-table_column_create.md) | Add a column to a data table |
| [`n8n data-table column delete`](n8n_data-table_column_delete.md) | Permanently delete a column and its data |
| [`n8n data-table column list`](n8n_data-table_column_list.md) | List the columns of a data table |
| [`n8n data-table column update`](n8n_data-table_column_update.md) | Rename or reorder a column |
| [`n8n data-table column`](n8n_data-table_column.md) | Manage the columns of a data table |
| [`n8n data-table create`](n8n_data-table_create.md) | Create a data table |
| [`n8n data-table delete`](n8n_data-table_delete.md) | Permanently delete a data table and every row in it |
| [`n8n data-table get`](n8n_data-table_get.md) | Show one data table and its columns |
| [`n8n data-table list`](n8n_data-table_list.md) | List data tables |
| [`n8n data-table row clear`](n8n_data-table_row_clear.md) | Permanently delete every row of a data table |
| [`n8n data-table row delete`](n8n_data-table_row_delete.md) | Permanently delete the rows matching a filter |
| [`n8n data-table row insert`](n8n_data-table_row_insert.md) | Insert rows into a data table |
| [`n8n data-table row list`](n8n_data-table_row_list.md) | List the rows of a data table |
| [`n8n data-table row update`](n8n_data-table_row_update.md) | Update every row matching a filter |
| [`n8n data-table row upsert`](n8n_data-table_row_upsert.md) | Update the row matching a filter, or insert it |
| [`n8n data-table row`](n8n_data-table_row.md) | Read and change the rows of a data table |
| [`n8n data-table update`](n8n_data-table_update.md) | Rename a data table |
| [`n8n data-table`](n8n_data-table.md) | Manage data tables, their rows and their columns |
| [`n8n discover`](n8n_discover.md) | List the API capabilities available to the current credential |
| [`n8n evaluation cancel`](n8n_evaluation_cancel.md) | Cancel a new or running evaluation |
| [`n8n evaluation case list`](n8n_evaluation_case_list.md) | List per-case results for an evaluation run |
| [`n8n evaluation case`](n8n_evaluation_case.md) | Inspect cases within an evaluation run |
| [`n8n evaluation create`](n8n_evaluation_create.md) | Start an asynchronous workflow evaluation |
| [`n8n evaluation get`](n8n_evaluation_get.md) | Show one evaluation run and its aggregate result |
| [`n8n evaluation list`](n8n_evaluation_list.md) | List evaluation runs for a workflow |
| [`n8n evaluation`](n8n_evaluation.md) | Run and inspect workflow evaluations |
| [`n8n execution delete`](n8n_execution_delete.md) | Permanently delete one stored execution |
| [`n8n execution get`](n8n_execution_get.md) | Show one execution |
| [`n8n execution list`](n8n_execution_list.md) | List executions with status, workflow and time filters |
| [`n8n execution retry`](n8n_execution_retry.md) | Retry a failed execution as a new run |
| [`n8n execution stop-many`](n8n_execution_stop-many.md) | Stop active executions matching filters |
| [`n8n execution stop`](n8n_execution_stop.md) | Stop one queued, running, or waiting execution |
| [`n8n execution tag list`](n8n_execution_tag_list.md) | List annotation tags on one execution |
| [`n8n execution tag set`](n8n_execution_tag_set.md) | Replace annotation tags on one execution |
| [`n8n execution tag`](n8n_execution_tag.md) | Read and replace annotation tags on an execution |
| [`n8n execution`](n8n_execution.md) | Inspect, stop, retry and delete workflow executions |
| [`n8n folder create`](n8n_folder_create.md) | Create a folder in a project |
| [`n8n folder delete`](n8n_folder_delete.md) | Delete a folder and move or archive what it holds |
| [`n8n folder get`](n8n_folder_get.md) | Show one folder with its totals |
| [`n8n folder list`](n8n_folder_list.md) | List the folders of a project |
| [`n8n folder update`](n8n_folder_update.md) | Rename a folder or move it under another folder |
| [`n8n folder`](n8n_folder.md) | Manage the folders of a project |
| [`n8n insight summary`](n8n_insight_summary.md) | Show aggregate execution insights |
| [`n8n insight`](n8n_insight.md) | Read aggregate execution insights |
| [`n8n ldap get`](n8n_ldap_get.md) | Get LDAP configuration |
| [`n8n ldap set`](n8n_ldap_set.md) | Fully replace LDAP configuration |
| [`n8n ldap sync history`](n8n_ldap_sync_history.md) | List LDAP synchronization history |
| [`n8n ldap sync run`](n8n_ldap_sync_run.md) | Run LDAP synchronization |
| [`n8n ldap sync`](n8n_ldap_sync.md) | Inspect or run LDAP synchronization |
| [`n8n ldap`](n8n_ldap.md) | Manage licensed LDAP settings and synchronization |
| [`n8n log-stream destination create`](n8n_log-stream_destination_create.md) | Create a log streaming destination |
| [`n8n log-stream destination delete`](n8n_log-stream_destination_delete.md) | Delete a log streaming destination |
| [`n8n log-stream destination get`](n8n_log-stream_destination_get.md) | Get a log streaming destination |
| [`n8n log-stream destination list`](n8n_log-stream_destination_list.md) | List configured log streaming destinations |
| [`n8n log-stream destination test`](n8n_log-stream_destination_test.md) | Send one test message to a destination |
| [`n8n log-stream destination update`](n8n_log-stream_destination_update.md) | Fully replace a log streaming destination |
| [`n8n log-stream destination`](n8n_log-stream_destination.md) | Create, inspect, replace, test, or delete destinations |
| [`n8n log-stream event-type list`](n8n_log-stream_event-type_list.md) | List event names available for streaming |
| [`n8n log-stream event-type`](n8n_log-stream_event-type.md) | Inspect streamable event types |
| [`n8n log-stream`](n8n_log-stream.md) | Manage licensed log streaming destinations |
| [`n8n node-policy instance get`](n8n_node-policy_instance_get.md) | Get the instance node type policy |
| [`n8n node-policy instance set`](n8n_node-policy_instance_set.md) | Fully replace the instance node type policy |
| [`n8n node-policy instance`](n8n_node-policy_instance.md) | Manage the instance node type policy |
| [`n8n node-policy project get`](n8n_node-policy_project_get.md) | Get a project's node type policy |
| [`n8n node-policy project set`](n8n_node-policy_project_set.md) | Fully replace a project's node type policy |
| [`n8n node-policy project`](n8n_node-policy_project.md) | Manage a project's node type policy |
| [`n8n node-policy`](n8n_node-policy.md) | Manage node type availability policies |
| [`n8n otel get`](n8n_otel_get.md) | Get effective OpenTelemetry settings |
| [`n8n otel set`](n8n_otel_set.md) | Fully replace OpenTelemetry settings |
| [`n8n otel test-trace`](n8n_otel_test-trace.md) | Send one test span to an OTLP collector |
| [`n8n otel`](n8n_otel.md) | Manage OpenTelemetry tracing settings |
| [`n8n project create`](n8n_project_create.md) | Create a project |
| [`n8n project delete`](n8n_project_delete.md) | Permanently delete a project |
| [`n8n project list`](n8n_project_list.md) | List projects |
| [`n8n project update`](n8n_project_update.md) | Rename a project |
| [`n8n project user add`](n8n_project_user_add.md) | Add one or more users to a project |
| [`n8n project user list`](n8n_project_user_list.md) | List the members of a project |
| [`n8n project user remove`](n8n_project_user_remove.md) | Remove a member from a project |
| [`n8n project user role set`](n8n_project_user_role_set.md) | Set a member's project role |
| [`n8n project user role`](n8n_project_user_role.md) | Manage members' project roles |
| [`n8n project user`](n8n_project_user.md) | Manage project members |
| [`n8n project`](n8n_project.md) | Manage projects and their members |
| [`n8n role create`](n8n_role_create.md) | Create a custom role |
| [`n8n role delete`](n8n_role_delete.md) | Permanently delete a custom role |
| [`n8n role get`](n8n_role_get.md) | Get one role |
| [`n8n role list`](n8n_role_list.md) | List global and project roles |
| [`n8n role update`](n8n_role_update.md) | Fully replace a custom role |
| [`n8n role`](n8n_role.md) | Manage global and project roles |
| [`n8n security-policy get`](n8n_security-policy_get.md) | Get effective security policy |
| [`n8n security-policy set`](n8n_security-policy_set.md) | Fully replace security policy |
| [`n8n security-policy`](n8n_security-policy.md) | Manage instance security policy |
| [`n8n tag create`](n8n_tag_create.md) | Create a tag |
| [`n8n tag delete`](n8n_tag_delete.md) | Permanently delete a tag |
| [`n8n tag get`](n8n_tag_get.md) | Get one tag |
| [`n8n tag list`](n8n_tag_list.md) | List tags |
| [`n8n tag update`](n8n_tag_update.md) | Rename a tag |
| [`n8n tag`](n8n_tag.md) | Manage workflow tags |
| [`n8n user create`](n8n_user_create.md) | Invite one or more users |
| [`n8n user delete`](n8n_user_delete.md) | Permanently delete a user |
| [`n8n user get`](n8n_user_get.md) | Get one user by ID or email |
| [`n8n user list`](n8n_user_list.md) | List instance or project users |
| [`n8n user role set`](n8n_user_role_set.md) | Set a user's global role |
| [`n8n user role`](n8n_user_role.md) | Manage users' global roles |
| [`n8n user`](n8n_user.md) | Manage instance users and global roles |
| [`n8n variable create`](n8n_variable_create.md) | Create a variable |
| [`n8n variable delete`](n8n_variable_delete.md) | Permanently delete a variable |
| [`n8n variable list`](n8n_variable_list.md) | List variables and their values |
| [`n8n variable update`](n8n_variable_update.md) | Fully replace a variable |
| [`n8n variable`](n8n_variable.md) | Manage instance and project variables |
| [`n8n version`](n8n_version.md) | Print build information |
| [`n8n workflow archive`](n8n_workflow_archive.md) | Archive a workflow, the reversible soft delete |
| [`n8n workflow create`](n8n_workflow_create.md) | Create a workflow from a definition or as an empty one |
| [`n8n workflow delete`](n8n_workflow_delete.md) | Permanently delete a workflow |
| [`n8n workflow get`](n8n_workflow_get.md) | Show one workflow with its definition |
| [`n8n workflow history`](n8n_workflow_history.md) | List the saved versions of a workflow |
| [`n8n workflow list`](n8n_workflow_list.md) | List workflows |
| [`n8n workflow publish`](n8n_workflow_publish.md) | Publish a workflow so its triggers run |
| [`n8n workflow tag list`](n8n_workflow_tag_list.md) | List the tags attached to a workflow |
| [`n8n workflow tag set`](n8n_workflow_tag_set.md) | Replace the tags of a workflow |
| [`n8n workflow tag`](n8n_workflow_tag.md) | Read and replace the tags of a workflow |
| [`n8n workflow transfer`](n8n_workflow_transfer.md) | Move a workflow to another project |
| [`n8n workflow unarchive`](n8n_workflow_unarchive.md) | Restore an archived workflow |
| [`n8n workflow unpublish`](n8n_workflow_unpublish.md) | Take the published version of a workflow offline |
| [`n8n workflow update`](n8n_workflow_update.md) | Replace the definition of a workflow |
| [`n8n workflow version get`](n8n_workflow_version_get.md) | Show one stored version of a workflow |
| [`n8n workflow version`](n8n_workflow_version.md) | Read a stored version of a workflow |
| [`n8n workflow`](n8n_workflow.md) | Manage workflows, their versions and their tags |
| [`n8n`](n8n.md) | Command-line client for the n8n API |
