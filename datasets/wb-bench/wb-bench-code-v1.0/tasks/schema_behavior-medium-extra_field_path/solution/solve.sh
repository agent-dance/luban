#!/bin/bash
set -euo pipefail
workspace_dir="${WORKSPACE_DIR:-/workspace}"
if [ -d "$workspace_dir" ]; then cd "$workspace_dir"; fi
tmp_patch="$(mktemp)"
cleanup() { rm -f "$tmp_patch"; }
trap cleanup EXIT
cat > "$tmp_patch" <<'CODEBUDDY_PATCH'
diff --git a/jsonschema_like/extra.py b/jsonschema_like/extra.py
--- a/jsonschema_like/extra.py
+++ b/jsonschema_like/extra.py
@@ -25,7 +25,7 @@
             if key in props:
                 errors.extend(validate(value, props[key], path + [key], schema_path + ["properties", key]))
             elif schema.get("additionalProperties") is False:
-                errors.append(ValidationError("additional property not allowed", path, schema_path + ["additionalProperties"]))
+                errors.append(ValidationError("additional property not allowed", path + [key], schema_path + ["additionalProperties"]))
     if isinstance(instance, list) and "items" in schema:
         for index, item in enumerate(instance):
             errors.extend(validate(item, schema["items"], path + [index], schema_path + ["items"]))
CODEBUDDY_PATCH
git apply "$tmp_patch"
