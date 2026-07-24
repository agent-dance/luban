#!/bin/bash
set -euo pipefail
workspace_dir="${WORKSPACE_DIR:-/workspace}"
if [ -d "$workspace_dir" ]; then cd "$workspace_dir"; fi
tmp_patch="$(mktemp)"
cleanup() { rm -f "$tmp_patch"; }
trap cleanup EXIT
cat > "$tmp_patch" <<'CODEBUDDY_PATCH'
diff --git a/jsonschema_like/best.py b/jsonschema_like/best.py
--- a/jsonschema_like/best.py
+++ b/jsonschema_like/best.py
@@ -45,5 +45,33 @@
     return errors
 
 
+def _oneof_branch(error):
+    if "oneOf" not in error.schema_path:
+        return None
+    index = error.schema_path.index("oneOf")
+    if index + 1 < len(error.schema_path) and isinstance(error.schema_path[index + 1], int):
+        return error.schema_path[index + 1]
+    return None
+
+
+def _enum_mismatch_branches(errors):
+    branches = {branch for branch in (_oneof_branch(error) for error in errors) if branch is not None}
+    enum_branches = {_oneof_branch(error) for error in errors if "enum" in error.schema_path and _oneof_branch(error) is not None}
+    if enum_branches and enum_branches != branches:
+        return enum_branches
+    return set()
+
+
+def _rank(error, enum_mismatch_branches):
+    generic = error.message == "is not valid under any schema"
+    branch = _oneof_branch(error)
+    branch_penalty = branch in enum_mismatch_branches
+    priority = 0 if "enum" in error.schema_path else 1 if ("type" in error.schema_path or "required" in error.schema_path) else 2
+    return (generic, branch_penalty, -len(error.path), priority, tuple(str(part) for part in error.path), tuple(str(part) for part in error.schema_path), error.message)
+
+
 def best_match(errors):
-    return min(errors, key=lambda error: len(error.path)) if errors else None
+    if not errors:
+        return None
+    enum_mismatch_branches = _enum_mismatch_branches(errors)
+    return min(errors, key=lambda error: _rank(error, enum_mismatch_branches))
CODEBUDDY_PATCH
git apply "$tmp_patch"
