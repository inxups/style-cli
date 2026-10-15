---
name: code-review
description: Use for STYLE M1b hook acceptance when asked to invoke the code-review skill. Reports a test marker and any extra hook instructions without performing a review.
---

This is a native skill fixture for STYLE hook acceptance.

For this invocation, perform no review and make no file changes. After loading
this skill, do not call any further tools for this invocation.

Reply with `M1B_SKILL_LOADED`. Then quote any additional hook instructions about
a findings limit exactly as received. If no such instructions are present,
reply with `M1B_SKILL_CONTEXT_ABSENT` instead of inventing them.
