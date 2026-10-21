# Complete offline release requirements

User decision: distribute complete offline packages; do not make users download model/runtime after installation, particularly on restricted networks in China. Drop macOS Intel. Keep Windows/Linux x64 and ARM64 and macOS ARM64.

Implementation: native application builds plus pinned standalone Python and CPU dependencies; fixed model revision; Resources next to Windows/Linux executable and inside macOS bundle. Application discovers its own runtime before development environments. Release packaging validates assets/architecture and checks text/query/image inference after relocation with sockets denied. Missing dependencies or inference failures block publication. Branch CI stays lightweight and does not distribute stripped builds. All five complete archives/checksums/reports are required for publication.

Verification: packaging guards with synthetic fixtures; bundled resource-path regressions; native Go race/vet/frontend suite; workflow lint; real local macOS offline text/image inference and full packaging. Hosted Windows/Linux acceptance remains a separate required check when a GitHub repository is configured.
