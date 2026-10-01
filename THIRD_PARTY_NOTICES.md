# Third-party notices

Tailboard's own source is MIT licensed. Dependency licenses remain separate.
The original `tg-clipboard` attribution is in [NOTICE.md](NOTICE.md).

## Mac engine

[`github.com/coder/websocket`](https://github.com/coder/websocket), version
1.8.14, is licensed under the ISC License. Its notice follows:

```
Copyright (c) 2025 Coder

Permission to use, copy, modify, and distribute this software for any
purpose with or without fee is hereby granted, provided that the above
copyright notice and this permission notice appear in all copies.

THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
```

## Android

[OkHttp](https://github.com/square/okhttp/blob/master/LICENSE.txt), version 5.5.0,
and its Okio and Kotlin runtime dependencies are licensed under Apache 2.0.
AndroidX test libraries and JUnit are development/test dependencies; their
licenses remain with those packages. The Gradle wrapper is Apache 2.0 licensed.

Dependencies are downloaded by Go and Gradle rather than vendored into this
repository. Anyone distributing compiled builds must retain the license notices
required by the libraries included in those builds.
