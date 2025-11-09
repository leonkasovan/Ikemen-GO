#if __VERSION__ >= 450
#extension GL_ARB_shader_viewport_layer_array  : enable
#define COMPAT_TEXTURE texture
layout (constant_id = 0) const bool useJoint0 = false;
layout (constant_id = 1) const bool useJoint1 = false;
layout (constant_id = 2) const bool useVertColor = false;
struct Light
{
    vec3 direction;
    float range;

    vec3 color;
    float intensity;

    vec3 position;
    float innerConeCos;

    float outerConeCos;
    int type;

    float shadowBias;
    float shadowMapFar;
};
layout(binding = 0) uniform UniformBufferObject0 {
	mat4 lightMatrices[24];
	Light lights[4];
	vec4 layers[6];
};

layout(binding = 2) uniform UniformBufferObject2 {
	vec4 morphTargetWeight[2];
	vec4 morphTargetOffset;
	int numJoints,numTargets,morphTargetTextureDimension;
};

layout(binding = 3) uniform sampler2D jointMatrices;
layout(binding = 4) uniform sampler2D morphTargetValues;

layout(push_constant, std430) uniform u {
	mat4 model;
	int numVertices;
};

//gl_VertexID is not available in 1.2
layout(location = 0) in int vertexId;
layout(location = 1) in vec3 position;
layout(location = 2) in vec2 uv;
layout(location = 3) in vec4 vertColor;
layout(location = 4) in vec4 joints_0;
layout(location = 5) in vec4 joints_1;
layout(location = 6) in vec4 weights_0;
layout(location = 7) in vec4 weights_1;
layout(location = 0) out vec4 fragPos;
layout(location = 1) out float vColor;
layout(location = 2) out vec2 texcoord;
layout(location = 3) out flat int lightIndex;
#else
#if __VERSION__ >= 130
#define COMPAT_VARYING out
#define COMPAT_ATTRIBUTE in
#define COMPAT_TEXTURE texture
#else
#extension GL_EXT_gpu_shader4 : enable
#define COMPAT_VARYING varying 
#define COMPAT_ATTRIBUTE attribute 
#define COMPAT_TEXTURE texture2D
#endif
uniform mat4 model;
uniform sampler2D jointMatrices;
uniform sampler2D morphTargetValues;
uniform int morphTargetTextureDimension;
uniform int numJoints;
uniform int numTargets;
uniform vec4 morphTargetWeight[2];
uniform vec4 morphTargetOffset;
uniform int numVertices;

//gl_VertexID is not available in 1.2
COMPAT_ATTRIBUTE float vertexId;
COMPAT_ATTRIBUTE vec3 position;
COMPAT_ATTRIBUTE vec4 vertColor;
COMPAT_ATTRIBUTE vec2 uv;
COMPAT_ATTRIBUTE vec4 joints_0;
COMPAT_ATTRIBUTE vec4 joints_1;
COMPAT_ATTRIBUTE vec4 weights_0;
COMPAT_ATTRIBUTE vec4 weights_1;

// Proper output declarations
COMPAT_VARYING vec4 fragPos;
COMPAT_VARYING float vColor;
COMPAT_VARYING vec2 texcoord;

#define useJoint0 (weights_0.x+weights_0.y+weights_0.z+weights_0.w+weights_1.x+weights_1.y+weights_1.z+weights_1.w>0.0)
const bool useJoint1 = true;
const bool useVertColor = true;
#endif


mat4 getMatrixFromTexture(float index){
	mat4 mat;
#if __VERSION__ >= 450
	mat[0] = COMPAT_TEXTURE(jointMatrices,vec2(0.5/6.0,(index+0.5)/float(numJoints)));
	mat[1] = COMPAT_TEXTURE(jointMatrices,vec2(1.5/6.0,(index+0.5)/float(numJoints)));
	mat[2] = COMPAT_TEXTURE(jointMatrices,vec2(2.5/6.0,(index+0.5)/float(numJoints)));
#else
	// For older GLSL versions, use constant expressions where possible
	float yCoord = (index + 0.5) / float(numJoints);
	mat[0] = COMPAT_TEXTURE(jointMatrices,vec2(0.08333333, yCoord)); // 0.5/6.0 = 0.08333333
	mat[1] = COMPAT_TEXTURE(jointMatrices,vec2(0.25, yCoord));       // 1.5/6.0 = 0.25
	mat[2] = COMPAT_TEXTURE(jointMatrices,vec2(0.41666666, yCoord)); // 2.5/6.0 = 0.41666666
#endif
	mat[3] = vec4(0,0,0,1);
	return transpose(mat);
}

mat4 getJointMatrix(){
	mat4 ret = mat4(0.0);
	ret += weights_0.x*getMatrixFromTexture(joints_0.x);
	ret += weights_0.y*getMatrixFromTexture(joints_0.y);
	ret += weights_0.z*getMatrixFromTexture(joints_0.z);
	ret += weights_0.w*getMatrixFromTexture(joints_0.w);
	if(useJoint1){
		ret += weights_1.x*getMatrixFromTexture(joints_1.x);
		ret += weights_1.y*getMatrixFromTexture(joints_1.y);
		ret += weights_1.z*getMatrixFromTexture(joints_1.z);
		ret += weights_1.w*getMatrixFromTexture(joints_1.w);
	}
	if(ret == mat4(0.0)){
		return mat4(1.0);
	}
	return ret;
}

void main() {
	texcoord = uv;
	if(useVertColor) {
		vColor = vertColor.a;
	}else{
		vColor = 1.0;
	}
	vec4 pos = vec4(position, 1.0);
	if(morphTargetOffset[0] > 0.0){
		for(int idx = 0; idx < numTargets; ++idx)
		{
			float i = float(idx*numVertices+int(vertexId));
			vec2 xy = vec2((i+0.5)/float(morphTargetTextureDimension)-floor(i/float(morphTargetTextureDimension)),(floor(i/float(morphTargetTextureDimension))+0.5)/float(morphTargetTextureDimension));
			if(idx < int(morphTargetOffset[0])){
				pos += morphTargetWeight[idx/4][idx%4] * COMPAT_TEXTURE(morphTargetValues,xy);
			}else if(idx >= int(morphTargetOffset[2]) && idx < int(morphTargetOffset[3])){
				texcoord += morphTargetWeight[idx/4][idx%4] * vec2(COMPAT_TEXTURE(morphTargetValues,xy));
			}
		}
	}
	if(useJoint0){
		mat4 jointMatrix = getJointMatrix();
		fragPos = model * jointMatrix * pos;
	}else{
		fragPos = model * pos;
	}
	#if __VERSION__ >= 450
	gl_Layer = int(layers[gl_InstanceIndex/4][gl_InstanceIndex%4]);
	lightIndex = gl_Layer/6;
	gl_Position = lightMatrices[gl_Layer] * fragPos;
	#else
	// For non-450 versions, we need to set gl_Position manually
	gl_Position = fragPos; // Or whatever transformation you need
	#endif
}
