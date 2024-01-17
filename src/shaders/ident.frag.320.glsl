precision highp float;

uniform sampler2D Texture;
in vec2 texcoord;
out vec4 FragColor;

void main(void) {
    FragColor = texture(Texture, texcoord);
}