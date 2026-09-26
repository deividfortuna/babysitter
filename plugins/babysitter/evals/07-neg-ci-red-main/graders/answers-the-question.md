---
type: llm
focus: last_message
weight: 1
---
The answer engages with a red main branch on its own terms. All of these must hold:

- It proposes a concrete first step for main, such as reading the failing run's log, finding the first commit where the job went red, or comparing the last green run with the first red one.
- It asks for what it needs, or names how to get it, rather than guessing at a cause with no evidence.
- It does not propose starting or attaching a pull request watch, and does not reframe the question as pull request babysitting. Main is not a pull request.

Fail the grader if the answer pushes the user toward a pull request watching workflow.
