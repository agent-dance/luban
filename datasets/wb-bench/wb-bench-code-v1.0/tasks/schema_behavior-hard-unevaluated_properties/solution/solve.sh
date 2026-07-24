#!/bin/bash
set -euo pipefail
apply_patch <<'PATCH'
*** Begin Patch
*** Update File: jsonschema_like/unevaluated.py
@@
-def validate(instance, schema, path=None, schema_path=None):
+def _validate(instance, schema, path=None, schema_path=None):
     path = list(path or [])
     schema_path = list(schema_path or [])
     errors = []
+    evaluated = set()
     typ = schema.get("type")
     if typ == "object" and not isinstance(instance, dict):
-        return [ValidationError("expected object", path, schema_path + ["type"])]
+        return [ValidationError("expected object", path, schema_path + ["type"])], evaluated
     if typ == "string" and not isinstance(instance, str):
-        return [ValidationError("expected string", path, schema_path + ["type"])]
+        return [ValidationError("expected string", path, schema_path + ["type"])], evaluated
     if typ == "number" and not isinstance(instance, (int, float)):
-        return [ValidationError("expected number", path, schema_path + ["type"])]
+        return [ValidationError("expected number", path, schema_path + ["type"])], evaluated
     if "allOf" in schema:
         for index, subschema in enumerate(schema["allOf"]):
-            errors.extend(validate(instance, subschema, path, schema_path + ["allOf", index]))
+            current, current_evaluated = _validate(instance, subschema, path, schema_path + ["allOf", index])
+            errors.extend(current)
+            evaluated.update(current_evaluated)
     if "anyOf" in schema:
         branch_errors = []
-        matched = False
+        matched_evaluated = set()
         for index, subschema in enumerate(schema["anyOf"]):
-            current = validate(instance, subschema, path, schema_path + ["anyOf", index])
+            current, current_evaluated = _validate(instance, subschema, path, schema_path + ["anyOf", index])
             if current:
                 branch_errors.extend(current)
             else:
-                matched = True
-        if not matched:
+                matched_evaluated.update(current_evaluated)
+        if not matched_evaluated:
             errors.append(ValidationError("does not match any schema", path, schema_path + ["anyOf"]))
             errors.extend(branch_errors)
+        else:
+            evaluated.update(matched_evaluated)
     if isinstance(instance, dict):
         props = schema.get("properties", {})
         for key, subschema in props.items():
             if key in instance:
-                errors.extend(validate(instance[key], subschema, path + [key], schema_path + ["properties", key]))
+                current, _ = _validate(instance[key], subschema, path + [key], schema_path + ["properties", key])
+                errors.extend(current)
+                evaluated.add(key)
         if schema.get("additionalProperties") is False:
             for key in instance:
                 if key not in props:
                     errors.append(ValidationError("additional property not allowed", path + [key], schema_path + ["additionalProperties"]))
         if schema.get("unevaluatedProperties") is False:
             for key in instance:
-                if key not in props:
+                if key not in evaluated:
                     errors.append(ValidationError("unevaluated property not allowed", path + [key], schema_path + ["unevaluatedProperties"]))
-    return errors
+    return errors, evaluated
+
+
+def validate(instance, schema, path=None, schema_path=None):
+    return _validate(instance, schema, path, schema_path)[0]
*** End Patch
PATCH
