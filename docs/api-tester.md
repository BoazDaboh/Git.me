# API tester

Open **Debug & tests > API requests**. Choose a project, then a service or **Custom address**.

1. Start your API in its terminal or through Git/Me's local service controls. The tester does not start servers.
2. Choose **Local** or **Production** in Address. Local uses the saved URL/port; Production reads the selected service's linked Vercel production target, or uses its recorded production address. If none is available, enter one manually. Edit the endpoint path to match a route your backend defines.
3. Choose GET to retrieve data, or POST to send JSON. GET uses **Send GET**; POST uses **Review POST > Confirm POST**. Read the HTTP status, elapsed time, body and response headers. JSON is formatted for reading; HTML is displayed as text.
4. Use **Stop request** or **Stop waiting** to cancel waiting. Requests stop after ten seconds. No request runs automatically when switching projects, opening a saved request or refreshing the app. A stopped POST may already have been processed.
5. Give the request a name and click **Save request** to reuse its URL path for this project.

Examples:

| Backend | URL example |
| --- | --- |
| Local Go API | `http://127.0.0.1:8080/communities` |
| Local PHP site | `http://localhost/my-project/api/communities.php` |
| Local Node API | `http://localhost:4000/communities` |
| Deployed backend | `https://api.example.com/communities` |

Routes such as `/communities` are examples you implement in your backend; they are not created by Go or Git/Me. Your backend must support the chosen method: a GET handler alone does not create a POST endpoint. Saved requests are reusable bookmarks, not scheduled tests. Choosing **Saved URL** restores the exact recorded address without automatically replacing it with a newer deployment URL.

GET asks the endpoint for data. A `404` means the server answered but the route was not found. `401`/`403` mean authentication or permission is required. **No response** means Git/Me did not receive an HTTP response; check the address, running server, network or HTTPS certificate.

## JSON POST

Select POST, enter valid JSON up to 64 KiB, then click **Review POST**. Check the exact URL and data before confirming. The request uses `Content-Type: application/json`. Review and cancel contact no target endpoint; confirmation sends the captured request once.

```json
{
  "name": "Ikeja"
}
```

Reviews expire after five minutes and can be used once. Changed service settings, cancellation or companion restart require another review. Request headers and JSON remain only in bounded, expiring companion memory until confirmation or cancellation.

A POST may create or change real data. If its response is lost or you stop waiting, the backend may already have processed it. **Unconfirmed** means check the target before sending again. Git/Me never retries a POST automatically. An HTTP error is still a response from your API; `405` usually means that method is unsupported.

## Headers and saved requests

Open **Headers** and enter one header per line:

```text
Accept: application/json
Authorization: Bearer YOUR_API_TOKEN
```

Headers are sent only to the URL you explicitly submit. Git/Me does not reuse your browser login, GitHub/Vercel credentials or environment files. Sensitive response headers such as Set-Cookie are masked.

Saved records contain the name, GET/POST method, URL path, service ID, timestamp and optional expected HTTP status. Query values, headers, JSON bodies, review tokens and responses are not written to SQLite, browser caches or exports. Switching service, address destination or project, or reloading, clears those session values. Reopening a saved POST requires entering its JSON and credentials again. You can keep up to 100 saved requests per project; imported records are validated too. Existing GET records remain compatible.

## Passed / Failed status checks

For a GET request, set **Expected HTTP status** to a whole number from 200 to 599, then save. The default is 200, including for older requests without an expectation. A normal Send GET also compares its response against the expectation captured when sent.

Click **Run saved GET checks** to check this project's saved GET URLs, one at a time. The table shows expected status, actual status, Passed/Failed, response time and check time. An expected 404 can pass; an unexpected 200 can fail. No response is Failed. A companion or saved-configuration error is Not run, rather than a claimed result from the target.

The runner reads saved request IDs from SQLite. It does not use the current editor's URL, headers, cookies or query values, and it never includes POST requests. Use normal Send GET for tests that need session authentication headers. Saved URLs can include local and production endpoints, so check the saved addresses before running the group.

**Stop checks** cancels the active request and the remaining queue. Changing projects or leaving API requests also stops it. Opening the page, reconnecting or reloading does not run checks automatically. Results remain in memory for this session and clear on reload; editing/removing a saved definition marks its old result Changed. Open a result's request to inspect or edit it, then explicitly rerun for a current result.

These assertions compare HTTP status only. They do not verify response JSON or prove the app's logic is correct. The saved-check runner stops after response headers and does not return or retain response bodies. No new SQLite schema migration is required.

## Current limits

- GET, expected-status checks and reviewed JSON POST are supported. JSON-body assertions, PUT/PATCH/DELETE, uploads and scheduling come later.
- Local, LAN and public HTTP(S) APIs work through the local companion, so the target does not need browser CORS permissions for this tester.
- Redirects are returned without being followed. HTTPS certificates must be valid. Downloads/binary bodies are not previewed.
- Body preview is limited to 1 MiB; response headers and request headers are bounded. At most four requests run concurrently.
- Git/Me's own ports and cloud metadata/link-local addresses are blocked. DNS destinations are checked before connecting.

The existing workspace format stores request templates inside each project's record. No new SQLite schema migration is required.

## Service setup from Debug & tests

Projects without a service profile show **Create service profile** in the debug area. The link opens that project's new-service form directly. Review the type, stack, local folder, address and commands, then save. Opening or cancelling the form does not create a profile or run an app.

Local services shows processes started through Git/Me and their output. Checks detects available commands to copy into a terminal; it does not execute the test suite. API requests sends explicit GET or reviewed JSON POST requests to your running local or deployed API. A profile supplies convenient local addresses and process controls; a custom API URL can still be tested without one.

