# Third-party components

Syranet's original Go source and documentation use the MIT license in LICENSE. Go modules retain their own licenses; versions and checksums are recorded in go.mod/go.sum.

The node image contains a separate sing-box executable from the official `ghcr.io/sagernet/sing-box:v1.14.3` image. sing-box is licensed under GPL-3.0-or-later; its license is reproduced in licenses/SINGBOX-LICENSE. The matching upstream source is https://github.com/SagerNet/sing-box/tree/v1.14.3 . This is not an MIT-licensed replacement for sing-box. Preserve upstream notices and the source/license information when distributing the node image.

PostgreSQL and the base image packages retain their own licenses. No third-party executable is committed to this repository. The optional download helper verifies the official sing-box release asset digest before extraction.
