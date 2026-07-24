#!/bin/bash
set -euo pipefail
workspace_dir="${WORKSPACE_DIR:-/workspace}"
if [ -d "$workspace_dir" ]; then cd "$workspace_dir"; fi
tmp_patch="$(mktemp)"
cleanup() { rm -f "$tmp_patch"; }
trap cleanup EXIT
cat > "$tmp_patch" <<'CODEBUDDY_PATCH'
diff --git a/jsonschema_like/validator.py b/jsonschema_like/validator.py
index 232b9be..6cbf574 100644
--- a/jsonschema_like/validator.py
+++ b/jsonschema_like/validator.py
@@ -1,3 +1,20 @@
 class ValidationError(Exception):
     def __init__(self, message, path=None, schema_path=None): super().__init__(message); self.message=message; self.path=list(path or []); self.schema_path=list(schema_path or [])
-def validate(instance, schema): return []
+def _err(msg,path,spath): return ValidationError(msg,path,spath)
+def _check(instance, schema, path, spath):
+    errors=[]; typ=schema.get("type")
+    if typ and typ=="object" and not isinstance(instance, dict): return [_err("is not of type object",path,spath+["type"])]
+    if typ and typ=="array" and not isinstance(instance, list): return [_err("is not of type array",path,spath+["type"])]
+    if typ and typ=="string" and not isinstance(instance, str): return [_err("is not of type string",path,spath+["type"])]
+    if typ and typ=="number" and not isinstance(instance, (int,float)) or isinstance(instance,bool): return [_err("is not of type number",path,spath+["type"])]
+    if "enum" in schema and instance not in schema["enum"]: errors.append(_err("is not one of enum",path,spath+["enum"]))
+    if isinstance(instance, dict):
+        for req in schema.get("required",[]):
+            if req not in instance: errors.append(_err(f"'{req}' is a required property",path,spath+["required"]))
+        for key, subschema in schema.get("properties",{}).items():
+            if key in instance: errors.extend(_check(instance[key],subschema,path+[key],spath+["properties",key]))
+    if isinstance(instance, list) and "items" in schema:
+        for i,item in enumerate(instance): errors.extend(_check(item,schema["items"],path+[i],spath+["items"]))
+    return errors
+def validate(instance, schema): return _check(instance,schema,[],[])
+def best_match(errors): return min(errors, key=lambda e: len(e.path)) if errors else None
CODEBUDDY_PATCH
git apply "$tmp_patch"
