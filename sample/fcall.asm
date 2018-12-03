.model small

.stack 100h

.code
f:
  mov ax,0x4c07
  ret

start:
  call near ptr f
  int 21h

end start
