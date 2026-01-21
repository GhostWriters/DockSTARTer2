# DockSTARTer 2

DockSTARTer makes it easy to install and manage Docker and Docker Compose on your machine. This is the new, Go-based version of the project, replacing the original Bash scripts for improved performance, maintainability, and cross-platform support.

## Getting Started

### Prerequisites

- Go 1.21 or higher
- Git

### Installation

Clone the repository:

```bash
git clone https://github.com/GhostWriters/DockSTARTer2.git
cd DockSTARTer2
```

### Running

To run the TUI directly from source:

```bash
go run .
```

To build and run:

```bash
go build -o ds2
./ds2
```

## Usage

```bash
# Launch the Main Menu (TUI)
./ds2

# Launch directly into the application installation menu
./ds2 -i

# Update the application
./ds2 -u
```

See `./ds2 --help` for all available commands and flags.

## Contributing

Please read [CONTRIBUTING.md](.github/CONTRIBUTING.md) for details on our code of conduct and the process for submitting pull requests.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
