# Goals

Heliosian is a small web application platform for the Helios school community (K-8). It hosts the community's apps — previously run on a no-code platform — as one service, built and maintained in the open by community volunteers.

## What it hosts

The first app is a school directory:

- Names, addresses, phone numbers
- Individual and family photos
- Name pronunciation

More apps follow over time, all served by the same binary.

## Principles

- **One static Go binary.** The server is Go, compiled to a static binary, running on Google Cloud Run. New apps are added to the same binary rather than deployed as new services.
- **Frameworkless client.** Client code is plain JavaScript (TypeScript is an acceptable evolution) with no client-side framework.
- **Great local development.** A single command runs the server locally. Content changes reload without restarting the server. Non-production sample data makes it possible to develop and test without touching real community data.
- **Agent-friendly repository.** Most development happens through coding agents driven by a wide community of contributors. The repo is structured, documented, and instrumented (including local screenshot capture) so agents can build, verify, and iterate without human hand-holding.
- **Open source, minimal configurability.** The code is public and specific to Helios; it does not aim to be a generic configurable product. It contains no secrets, credentials, or private community data.

## Data

- Structured data comes from Google Sheets. The system of record is Veracross, and a direct integration replaces Sheets when an API becomes available.
- Blobs (photos and other uploads) live in Google Drive.
- Existing data is imported from the current platforms.
