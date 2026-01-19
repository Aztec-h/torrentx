# BitTorrent Client (Go)

A simple BitTorrent client implemented in Go. This project demonstrates the core concepts of the BitTorrent protocol, including torrent file parsing, peer-to-peer communication, and piece downloading.

## Features
- **Bencode Parsing:** Decodes .torrent files using the bencode format.
- **Peer-to-Peer Networking:** Handles handshake and message exchange between peers.
- **Torrent Management:** Loads torrent metadata and coordinates piece downloads.
- **Modular Design:** Organized into packages for bencode, p2p, and torrent logic.

## Project Structure
```
.
├── bencode/         # Bencode parsing logic
│   └── bencode.go
├── p2p/             # Peer-to-peer protocol implementation
│   ├── handshake.go
│   └── message.go
├── torrent/         # Torrent file and worker logic
│   ├── torrent.go
│   └── worker.go
├── main.go          # Entry point for the client
├── go.mod           # Go module definition
├── README.md        # Project documentation
└── debian-13.3.0-amd64-netinst.iso.torrent # Example torrent file
```

## Getting Started
### Prerequisites
- Go 1.18 or newer

### Build & Run
1. Clone the repository:
   ```sh
   git clone <repo-url>
   cd Bittorent
   ```
2. Build the project:
   ```sh
   go build -o bittorrent main.go
   ```
3. Run the client:
   ```sh
   ./bittorrent debian-13.3.0-amd64-netinst.iso.torrent
   ```

## Usage
- Place your `.torrent` file in the project directory.
- Run the client with the torrent file as an argument.
- The client will connect to peers and begin downloading pieces.

## Code Overview
- **main.go:** Initializes the client and starts the download process.
- **bencode/**: Contains bencode decoding utilities.
- **p2p/**: Implements BitTorrent handshake and message protocols.
- **torrent/**: Manages torrent metadata and piece downloading.

## License
This project is licensed under the MIT License.

## Acknowledgements
- [BitTorrent Protocol Specification](https://wiki.theory.org/BitTorrentSpecification)
- [Go Programming Language](https://golang.org/)

---
Feel free to contribute or open issues for improvements!
