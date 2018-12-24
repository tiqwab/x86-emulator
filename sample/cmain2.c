// wasm -ms -ecc greeting.asm
// wcl -ecc -ms -s cmain2.c greeting.obj

void greeting();

void main(void)
{
  greeting();
}
