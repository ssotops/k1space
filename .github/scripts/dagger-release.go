name: Dagger Release Workflow

on:
  push:
    branches:
      - main
      - master

jobs:
  dagger-release:
    runs-on: ubuntu-latest
    permissions:
      contents: write  # This gives the workflow permission to create releases
    steps:
      - name: Checkout code
        uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.23.0'  # Using the latest stable version

      - name: Install Dagger
        run: |
          curl -L https://dl.dagger.io/dagger/install.sh | sh
          sudo mv bin/dagger /usr/local/bin
          dagger version

      - name: Build k1space
        run: |
          go build -o k1space
          sudo mv k1space /usr/local/bin

      - name: Run Dagger workflow
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          cd .github/scripts
          go mod init dagger-release
          go mod edit -replace github.com/ssotspace/k1space=../..
          go get dagger.io/dagger@latest
          go mod tidy
          go run dagger-release.go
