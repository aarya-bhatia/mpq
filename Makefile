all: mpq

mpq: $(wildcard *.go)
	go build -o mpq

install: mpq
	mkdir -p $(HOME)/.local/bin
	mv mpq $(HOME)/.local/bin/mpq

uninstall:
	rm -rf $(HOME)/.local/bin/mpq

tags: $(wildcard *.go)
	gotags -R . > $@

clean:
	rm -rf mpq

