.model small
.386p
.stack 100h

.code
start:
  ; do 'mov ax,4c01h'
  mov cx,4ch
  shl cx,8
  add cx,01h
  mov ax,cx
  mov cx,ax
  ;
  int 21h

end start


