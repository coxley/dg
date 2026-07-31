# Native clipboard

This package derives from `golang.design/x/clipboard` v0.8.0 under its included
MIT license. It lives internally because dg needs one atomic clipboard item with
plain-text and application-specific representations. The upstream API writes
only one representation at a time.

Keep platform fixes synchronized with upstream until it exposes an equivalent
multi-format operation.
