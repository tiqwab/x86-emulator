.model small

.stack 100h

.data
msg db 'Hello world!$'

.code
greeting:
  push bp
  mov bp,sp

  mov ah,09h
  mov dx,[bp+4]
  int 21h

  pop bp
  ret

start:
  push bx
  push cx
  push dx
  push si
  push di
  push bp
  mov bp,sp

  mov ax,seg msg
  mov ds,ax

  sub sp,0x0002
  lea ax,msg
  push ax
  call near ptr greeting
  add sp,0x0002

  mov sp,bp
  pop bp
  pop di
  pop si
  pop dx
  pop cx
  pop bx
  mov ax,0x4c00
  int 21h

end start
