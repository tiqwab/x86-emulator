.model small
.386p
.stack 100h

.data
msg db 'Hello world!$'

.code
start:
  mov ax,seg msg
  mov ds,ax
  ;
  mov ah,09h
  lea dx,msg
  int 21h
  ;
  mov ax,4c00h
  int 21h

end start
