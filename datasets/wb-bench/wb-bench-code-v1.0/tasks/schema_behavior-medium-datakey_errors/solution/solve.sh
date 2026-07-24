#!/bin/bash
set -euo pipefail
workspace_dir="${WORKSPACE_DIR:-/workspace}"
if [ -d "$workspace_dir" ]; then cd "$workspace_dir"; fi
tmp_patch="$(mktemp)"
cleanup() { rm -f "$tmp_patch"; }
trap cleanup EXIT
cat > "$tmp_patch" <<'CODEBUDDY_PATCH'
diff --git a/marshmallow_like/datakey.py b/marshmallow_like/datakey.py
--- a/marshmallow_like/datakey.py
+++ b/marshmallow_like/datakey.py
@@ -31,19 +31,19 @@
             input_key = self._input_key(name, field)
             if input_key not in data:
                 if field.required:
-                    errors.append({"loc": path + [name], "msg": "missing"})
+                    errors.append({"loc": path + [input_key], "msg": "missing"})
                 continue
             value = data[input_key]
             if isinstance(field, Nested):
                 if field.many:
                     for index, item in enumerate(value):
-                        errors.extend(field.schema.load(item, path + [name, index])[1])
+                        errors.extend(field.schema.load(item, path + [input_key, index])[1])
                 else:
-                    nested, nested_errors = field.schema.load(value, path + [name])
+                    nested, nested_errors = field.schema.load(value, path + [input_key])
                     output[name] = nested
                     errors.extend(nested_errors)
             elif not isinstance(value, field.typ):
-                errors.append({"loc": path + [name], "msg": "invalid"})
+                errors.append({"loc": path + [input_key], "msg": "invalid"})
             else:
                 output[name] = value
         return output, errors
CODEBUDDY_PATCH
git apply "$tmp_patch"
