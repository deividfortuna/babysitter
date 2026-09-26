---
max_turns: 8
timeout_seconds: 180
allowed_tools: [Skill, Read]
model: opus
runs: 3
---
Review my staged diff for bugs before I commit it. Nothing is pushed yet.

```diff
--- a/internal/store/activity.go
+++ b/internal/store/activity.go
@@ -41,10 +41,14 @@ func (s *Store) Recent(watchID string, limit int) ([]Activity, error) {
-	rows, err := s.db.Query(`SELECT id, kind, payload FROM activity WHERE watch_id = ? ORDER BY id DESC LIMIT ?`, watchID, limit)
+	q := fmt.Sprintf(`SELECT id, kind, payload FROM activity WHERE watch_id = '%s' ORDER BY id DESC LIMIT %d`, watchID, limit)
+	rows, err := s.db.Query(q)
 	if err != nil {
 		return nil, err
 	}
-	defer rows.Close()
 
 	var out []Activity
 	for rows.Next() {
```
