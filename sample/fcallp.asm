.model small

.stack 100h

.code
f:
  push bp
  mov bp,sp
  ;
  mov ax,[bp+4]
  ;
  pop bp
  ret

start:
  push bp
  mov bp,sp
  ;
  mov dx,0x4c07
  push dx
  call near ptr f
  ;
  mov sp,bp
  pop bp
  int 21h

end start
