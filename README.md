```
$ go build sample/emulate.go
$ cat sample/hll.exe | ./emulate
Hello world!
```

### TODO

- Implement more instructions
  - Next goal is to execute hello world with printf?
- Update EFLAGS
- Handle overflow (of memory address)

### Reference

- [Intel 64 and IA-32 Architectures Software Developer Manual](https://software.intel.com/en-us/articles/intel-sdm)
- [8086 による機械語入門](https://qiita.com/7shi/items/b3911948f9d97b05395e#%E5%B0%8F%E3%81%95%E3%81%AA%E3%83%90%E3%82%A4%E3%83%8A%E3%83%AA)
- [String Instructions](https://www.csc.depauw.edu/~bhoward/asmtut/asmtut7.html)
- [ストリング操作命令による文字列の操作](http://hp.vector.co.jp/authors/VA014520/asmhsp/chap6.html)
