---
id: no-todo-comments
target: tool
tool: [file.edit, file.write]
action: warn
repeat: every
when:
  conditions:
    - {field: tool.added, op: contains, pattern: TODO}
---
Do not leave TODO comments in code. Report unfinished work in the response.
