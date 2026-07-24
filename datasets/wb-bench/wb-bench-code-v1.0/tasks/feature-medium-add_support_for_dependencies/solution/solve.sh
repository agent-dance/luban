#!/bin/bash
set -euo pipefail
workspace_dir="${WORKSPACE_DIR:-/workspace}"
cd "$workspace_dir"
tmp_patch="$(mktemp)"
cleanup() { rm -f "$tmp_patch"; }
trap cleanup EXIT
cat > "$tmp_patch" <<'CODEBUDDY_PATCH'
diff --git a/fastapi/applications.py b/fastapi/applications.py
index d5ea1d72..298aca92 100644
--- a/fastapi/applications.py
+++ b/fastapi/applications.py
@@ -401,15 +401,34 @@ class FastAPI(Starlette):
         return decorator
 
     def add_api_websocket_route(
-        self, path: str, endpoint: Callable[..., Any], name: Optional[str] = None
+        self,
+        path: str,
+        endpoint: Callable[..., Any],
+        name: Optional[str] = None,
+        *,
+        dependencies: Optional[Sequence[Depends]] = None,
     ) -> None:
-        self.router.add_api_websocket_route(path, endpoint, name=name)
+        self.router.add_api_websocket_route(
+            path,
+            endpoint,
+            name=name,
+            dependencies=dependencies,
+        )
 
     def websocket(
-        self, path: str, name: Optional[str] = None
+        self,
+        path: str,
+        name: Optional[str] = None,
+        *,
+        dependencies: Optional[Sequence[Depends]] = None,
     ) -> Callable[[DecoratedCallable], DecoratedCallable]:
         def decorator(func: DecoratedCallable) -> DecoratedCallable:
-            self.add_api_websocket_route(path, func, name=name)
+            self.add_api_websocket_route(
+                path,
+                func,
+                name=name,
+                dependencies=dependencies,
+            )
             return func
 
         return decorator
diff --git a/fastapi/routing.py b/fastapi/routing.py
index 7f1936f7..af628f32 100644
--- a/fastapi/routing.py
+++ b/fastapi/routing.py
@@ -296,13 +296,21 @@ class APIWebSocketRoute(routing.WebSocketRoute):
         endpoint: Callable[..., Any],
         *,
         name: Optional[str] = None,
+        dependencies: Optional[Sequence[params.Depends]] = None,
         dependency_overrides_provider: Optional[Any] = None,
     ) -> None:
         self.path = path
         self.endpoint = endpoint
         self.name = get_name(endpoint) if name is None else name
+        self.dependencies = list(dependencies or [])
         self.path_regex, self.path_format, self.param_convertors = compile_path(path)
         self.dependant = get_dependant(path=self.path_format, call=self.endpoint)
+        for depends in self.dependencies[::-1]:
+            self.dependant.dependencies.insert(
+                0,
+                get_parameterless_sub_dependant(depends=depends, path=self.path_format),
+            )
+
         self.app = websocket_session(
             get_websocket_app(
                 dependant=self.dependant,
@@ -416,10 +424,7 @@ class APIRoute(routing.Route):
         else:
             self.response_field = None  # type: ignore
             self.secure_cloned_response_field = None
-        if dependencies:
-            self.dependencies = list(dependencies)
-        else:
-            self.dependencies = []
+        self.dependencies = list(dependencies or [])
         self.description = description or inspect.cleandoc(self.endpoint.__doc__ or "")
         # if a "form feed" character (page break) is found in the description text,
         # truncate description text to the content preceding the first "form feed"
@@ -514,7 +519,7 @@ class APIRouter(routing.Router):
             ), "A path prefix must not end with '/', as the routes will start with '/'"
         self.prefix = prefix
         self.tags: List[Union[str, Enum]] = tags or []
-        self.dependencies = list(dependencies or []) or []
+        self.dependencies = list(dependencies or [])
         self.deprecated = deprecated
         self.include_in_schema = include_in_schema
         self.responses = responses or {}
@@ -688,21 +693,37 @@ class APIRouter(routing.Router):
         return decorator
 
     def add_api_websocket_route(
-        self, path: str, endpoint: Callable[..., Any], name: Optional[str] = None
+        self,
+        path: str,
+        endpoint: Callable[..., Any],
+        name: Optional[str] = None,
+        *,
+        dependencies: Optional[Sequence[params.Depends]] = None,
     ) -> None:
+        current_dependencies = self.dependencies.copy()
+        if dependencies:
+            current_dependencies.extend(dependencies)
+
         route = APIWebSocketRoute(
             self.prefix + path,
             endpoint=endpoint,
             name=name,
+            dependencies=current_dependencies,
             dependency_overrides_provider=self.dependency_overrides_provider,
         )
         self.routes.append(route)
 
     def websocket(
-        self, path: str, name: Optional[str] = None
+        self,
+        path: str,
+        name: Optional[str] = None,
+        *,
+        dependencies: Optional[Sequence[params.Depends]] = None,
     ) -> Callable[[DecoratedCallable], DecoratedCallable]:
         def decorator(func: DecoratedCallable) -> DecoratedCallable:
-            self.add_api_websocket_route(path, func, name=name)
+            self.add_api_websocket_route(
+                path, func, name=name, dependencies=dependencies
+            )
             return func
 
         return decorator
@@ -817,8 +838,16 @@ class APIRouter(routing.Router):
                     name=route.name,
                 )
             elif isinstance(route, APIWebSocketRoute):
+                current_dependencies = []
+                if dependencies:
+                    current_dependencies.extend(dependencies)
+                if route.dependencies:
+                    current_dependencies.extend(route.dependencies)
                 self.add_api_websocket_route(
-                    prefix + route.path, route.endpoint, name=route.name
+                    prefix + route.path,
+                    route.endpoint,
+                    dependencies=current_dependencies,
+                    name=route.name,
                 )
             elif isinstance(route, routing.WebSocketRoute):
                 self.add_websocket_route(
diff --git a/tests/test_ws_dependencies.py b/tests/test_ws_dependencies.py
--- a/tests/test_ws_dependencies.py
+++ b/tests/test_ws_dependencies.py
@@ -0,0 +1,70 @@
+import json
+from typing import List
+
+from fastapi import APIRouter, Depends, FastAPI, WebSocket
+from fastapi.testclient import TestClient
+from typing_extensions import Annotated
+
+
+def dependency_list() -> List[str]:
+    return []
+
+
+DepList = Annotated[List[str], Depends(dependency_list)]
+
+
+def create_dependency(name: str):
+    def fun(deps: DepList):
+        deps.append(name)
+
+    return Depends(fun)
+
+def create_app() -> FastAPI:
+    router = APIRouter(dependencies=[create_dependency("router")])
+    prefix_router = APIRouter(dependencies=[create_dependency("prefix_router")])
+    app = FastAPI(dependencies=[create_dependency("app")])
+
+    @app.websocket("/", dependencies=[create_dependency("index")])
+    async def index(websocket: WebSocket, deps: DepList):
+        await websocket.accept()
+        await websocket.send_text(json.dumps(deps))
+        await websocket.close()
+
+    @router.websocket("/router", dependencies=[create_dependency("routerindex")])
+    async def routerindex(websocket: WebSocket, deps: DepList):
+        await websocket.accept()
+        await websocket.send_text(json.dumps(deps))
+        await websocket.close()
+
+    @prefix_router.websocket("/", dependencies=[create_dependency("routerprefixindex")])
+    async def routerprefixindex(websocket: WebSocket, deps: DepList):
+        await websocket.accept()
+        await websocket.send_text(json.dumps(deps))
+        await websocket.close()
+
+    app.include_router(router, dependencies=[create_dependency("router2")])
+    app.include_router(
+        prefix_router, prefix="/prefix", dependencies=[create_dependency("prefix_router2")]
+    )
+    return app
+
+
+def test_index():
+    client = TestClient(create_app())
+    with client.websocket_connect("/") as websocket:
+        data = json.loads(websocket.receive_text())
+        assert data == ["app", "index"]
+
+
+def test_routerindex():
+    client = TestClient(create_app())
+    with client.websocket_connect("/router") as websocket:
+        data = json.loads(websocket.receive_text())
+        assert data == ["app", "router2", "router", "routerindex"]
+
+
+def test_routerprefixindex():
+    client = TestClient(create_app())
+    with client.websocket_connect("/prefix/") as websocket:
+        data = json.loads(websocket.receive_text())
+        assert data == ["app", "prefix_router2", "prefix_router", "routerprefixindex"]
CODEBUDDY_PATCH
git apply "$tmp_patch"
