.model small

.stack 100h

.code
start:
  mov cx,0x0010
  mov es,cx
  mov bx,0x0370
  mov es:[0x00b0],bx
  mov dx,es:[0x00b0]

  mov ax,0x4c00
  int 21h

end start
