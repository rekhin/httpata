# httpata

**httpata** is a command-line tool for making HTTP requests and transforming JSON responses using [JSONata](https://jsonata.org/). Inspired by `curl`, it provides advanced support for raw RFC 2616 HTTP requests and flexible JSON querying.

## Features

- Send HTTP requests using command-line flags or as a raw HTTP request (RFC 2616 format)
- Supports all HTTP methods (GET, POST, PUT, DELETE, etc.)
- Add custom headers and request bodies
- Read request body from a string, file, or stdin
- Transform and filter JSON responses via JSONata expressions
- Configurable timeout

## JSONata Guide

- Official site: [https://jsonata.org/](https://jsonata.org/)
- Quickstart and syntax guide: [JSONata Guide](https://github.com/tlcsdm/vscode-json-tree-view/blob/HEAD/docs/jsonata-guide.md)

## Installation

```sh
go build -o httpata main.go
```

## Usage

### Standard flag-based request

```sh
./httpata \
  -method POST \
  -url https://jsonplaceholder.typicode.com/posts \
  -H 'Content-Type: application/json' \
  -data '{"title": "foo", "body": "bar", "userId": 1}'
```

### Raw HTTP request file (RFC 2616)

```sh
./httpata -raw request.http
```

#### Example `request.http` file

```
POST https://jsonplaceholder.typicode.com/posts HTTP/1.1
Content-Type: application/json
Authorization: Bearer token123

{
  "title": "foo",
  "body": "bar",
  "userId": 1
}
```

### Raw HTTP request from stdin

If you call `httpata` without any flags or parameters, it will read an RFC 2616-compatible HTTP request from stdin until EOF, then execute it.

#### Example:

```sh
./httpata
```

Then, paste your raw HTTP request (for example):

```
GET https://jsonplaceholder.typicode.com/todos/1 HTTP/1.1
Authorization: Bearer token123
Content-Type: application/json

```
…and press `Ctrl+D` to send it.

### Transform JSON response with JSONata

```sh
./httpata -url https://jsonplaceholder.typicode.com/todos/1 -jsonata 'title'
```

See the [JSONata Guide](https://github.com/tlcsdm/vscode-json-tree-view/blob/HEAD/docs/jsonata-guide.md) for more on writing expressions.

## Flags

- `-method`        HTTP method (default: GET)
- `-url`           Request URL
- `-H`             Header (`Key: Value`; can be repeated)
- `-data`          Request body as a string
- `-data-file`     Path to file with request body (overrides `-data`)
- `-raw`           Path to a raw HTTP request file (`request.http`, RFC 2616). If not set and no other flags, reads from stdin.
- `-jsonata`       JSONata expression to transform the JSON response
- `-timeout`       Request timeout in seconds (default: 30)

## RFC 2616 Support

With the `-raw` flag or if you supply no flags at all, `httpata` expects a raw HTTP request conforming to [RFC 2616 (HTTP/1.1)](https://www.rfc-editor.org/rfc/rfc2616). This format is compatible with exports from tools like Postman or Burp Suite, or can be written by hand.

## License

MIT License. See [LICENSE](LICENSE) for details.
