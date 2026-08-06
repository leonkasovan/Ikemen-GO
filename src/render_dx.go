//go:build windows && !android && !armdevice

package main

import (
	"bytes"
	"container/list"
	_ "embed"
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
	"runtime"
	"sync"
	"unsafe"

	mgl "github.com/go-gl/mathgl/mgl32"
	"github.com/ikemen-engine/Ikemen-GO/packages/go-sdl2/sdl"
)

/*
#cgo LDFLAGS: -ld3d11 -ldxgi -ld3dcompiler
#include <windows.h>
#include <unknwn.h>
#include <d3d11.h>
#include <dxgi.h>
#include <dxgi1_2.h>
#include <d3dcompiler.h>
#include <stdio.h>
#include <string.h>

static char dx_err[2048];
static int dx_debug_active = 0;

int dx_debug_layer(void) { return dx_debug_active; }

const char* dx_last_error(void) { return dx_err; }

void* dx_create_device(int debug) {
	UINT flags = 0;
	if (debug) flags |= D3D11_CREATE_DEVICE_DEBUG;
	D3D_FEATURE_LEVEL levels[] = { D3D_FEATURE_LEVEL_11_1, D3D_FEATURE_LEVEL_11_0 };
	ID3D11Device* device = NULL;
	ID3D11DeviceContext* ctx = NULL;
	HRESULT hr = D3D11CreateDevice(NULL, D3D_DRIVER_TYPE_HARDWARE, NULL, flags, levels, 2, D3D11_SDK_VERSION, &device, NULL, &ctx);
	dx_debug_active = 0;
	if (FAILED(hr) && debug) {
		flags &= ~D3D11_CREATE_DEVICE_DEBUG;
		hr = D3D11CreateDevice(NULL, D3D_DRIVER_TYPE_HARDWARE, NULL, flags, levels, 2, D3D11_SDK_VERSION, &device, NULL, &ctx);
	} else if (SUCCEEDED(hr) && debug) {
		dx_debug_active = 1;
	}
	if (FAILED(hr)) {
		snprintf(dx_err, sizeof(dx_err), "D3D11CreateDevice failed hr=0x%08lx", hr);
		return NULL;
	}
	((IUnknown*)ctx)->lpVtbl->AddRef((IUnknown*)ctx);
	return device;
}

void dx_pull_messages(void* device) {
	ID3D11InfoQueue* q = NULL;
	HRESULT hr = ((ID3D11Device*)device)->lpVtbl->QueryInterface((ID3D11Device*)device, &IID_ID3D11InfoQueue, (void**)&q);
	if (FAILED(hr) || !q) return;
	UINT64 n = q->lpVtbl->GetNumStoredMessages(q);
	if (n > 60) n = 60;
	for (UINT64 i = 0; i < n; i++) {
		SIZE_T len = 0;
		q->lpVtbl->GetMessage(q, i, NULL, &len);
		if (len == 0) continue;
		D3D11_MESSAGE* msg = (D3D11_MESSAGE*)malloc(len);
		if (msg && SUCCEEDED(q->lpVtbl->GetMessage(q, i, msg, &len))) {
			fprintf(stderr, "[D3D11 %s] %s\n", msg->Severity == D3D11_MESSAGE_SEVERITY_ERROR ? "ERROR" : "WARN", msg->pDescription);
		}
		free(msg);
	}
	q->lpVtbl->ClearStoredMessages(q);
	q->lpVtbl->Release(q);
}

void* dx_get_context(void* device) {
	ID3D11DeviceContext* ctx = NULL;
	((ID3D11Device*)device)->lpVtbl->GetImmediateContext((ID3D11Device*)device, &ctx);
	return ctx;
}

void* dx_create_factory(void) {
	IDXGIFactory2* factory = NULL;
	HRESULT hr = CreateDXGIFactory1(&IID_IDXGIFactory2, (void**)&factory);
	if (FAILED(hr)) {
		snprintf(dx_err, sizeof(dx_err), "CreateDXGIFactory1 failed hr=0x%08lx", hr);
		return NULL;
	}
	return factory;
}

void* dx_create_swapchain(void* factory, void* device, void* hwnd, int w, int h) {
	DXGI_SWAP_CHAIN_DESC1 desc;
	memset(&desc, 0, sizeof(desc));
	desc.Width = (UINT)w;
	desc.Height = (UINT)h;
	desc.Format = DXGI_FORMAT_R8G8B8A8_UNORM;
	desc.Stereo = FALSE;
	desc.SampleDesc.Count = 1;
	desc.SampleDesc.Quality = 0;
	desc.BufferUsage = DXGI_USAGE_RENDER_TARGET_OUTPUT;
	desc.BufferCount = 2;
	desc.Scaling = DXGI_SCALING_STRETCH;
	desc.SwapEffect = DXGI_SWAP_EFFECT_FLIP_DISCARD;
	desc.AlphaMode = DXGI_ALPHA_MODE_IGNORE;
	IDXGISwapChain1* sc = NULL;
	HRESULT hr = ((IDXGIFactory2*)factory)->lpVtbl->CreateSwapChainForHwnd((IDXGIFactory2*)factory, (IUnknown*)device, (HWND)hwnd, &desc, NULL, NULL, &sc);
	if (FAILED(hr)) {
		snprintf(dx_err, sizeof(dx_err), "CreateSwapChainForHwnd failed hr=0x%08lx", hr);
		return NULL;
	}
	((IDXGIFactory2*)factory)->lpVtbl->MakeWindowAssociation((IDXGIFactory2*)factory, hwnd, DXGI_MWA_NO_ALT_ENTER);
	return sc;
}

int dx_resize(void* swapchain, int w, int h) {
	HRESULT hr = ((IDXGISwapChain*)swapchain)->lpVtbl->ResizeBuffers((IDXGISwapChain*)swapchain, 0, (UINT)w, (UINT)h, DXGI_FORMAT_UNKNOWN, 0);
	if (FAILED(hr)) {
		snprintf(dx_err, sizeof(dx_err), "ResizeBuffers failed hr=0x%08lx", hr);
		return -1;
	}
	return 0;
}

void* dx_get_backbuffer(void* swapchain) {
	ID3D11Texture2D* tex = NULL;
	HRESULT hr = ((IDXGISwapChain*)swapchain)->lpVtbl->GetBuffer((IDXGISwapChain*)swapchain, 0, &IID_ID3D11Texture2D, (void**)&tex);
	if (FAILED(hr)) {
		snprintf(dx_err, sizeof(dx_err), "GetBuffer failed hr=0x%08lx", hr);
		return NULL;
	}
	return tex;
}

void dx_present(void* swapchain, int interval) {
	((IDXGISwapChain*)swapchain)->lpVtbl->Present((IDXGISwapChain*)swapchain, interval, 0);
}

void* dx_create_texture(void* device, int w, int h, int fmt, int bindFlags, int msaa, int mips, int arraySize, int cube, int usage, int cpuAccess) {
	D3D11_TEXTURE2D_DESC desc;
	memset(&desc, 0, sizeof(desc));
	desc.Width = (UINT)w;
	desc.Height = (UINT)h;
	desc.MipLevels = (UINT)mips;
	desc.ArraySize = (UINT)arraySize;
	desc.Format = (DXGI_FORMAT)fmt;
	desc.SampleDesc.Count = (UINT)msaa;
	desc.SampleDesc.Quality = 0;
	desc.Usage = (D3D11_USAGE)usage;
	desc.BindFlags = (UINT)bindFlags;
	desc.CPUAccessFlags = (UINT)cpuAccess;
	if (cube) desc.MiscFlags |= D3D11_RESOURCE_MISC_TEXTURECUBE;
	ID3D11Texture2D* tex = NULL;
	HRESULT hr = ((ID3D11Device*)device)->lpVtbl->CreateTexture2D((ID3D11Device*)device, &desc, NULL, &tex);
	if (FAILED(hr)) {
		snprintf(dx_err, sizeof(dx_err), "CreateTexture2D(%dx%d fmt=%d) failed hr=0x%08lx", w, h, fmt, hr);
		return NULL;
	}
	return tex;
}

void* dx_create_rtv(void* device, void* resource, int msaa) {
	D3D11_RENDER_TARGET_VIEW_DESC desc;
	memset(&desc, 0, sizeof(desc));
	desc.ViewDimension = msaa > 1 ? D3D11_RTV_DIMENSION_TEXTURE2DMS : D3D11_RTV_DIMENSION_TEXTURE2D;
	ID3D11RenderTargetView* rtv = NULL;
	HRESULT hr = ((ID3D11Device*)device)->lpVtbl->CreateRenderTargetView((ID3D11Device*)device, (ID3D11Resource*)resource, &desc, &rtv);
	if (FAILED(hr)) {
		snprintf(dx_err, sizeof(dx_err), "CreateRenderTargetView failed hr=0x%08lx", hr);
		return NULL;
	}
	return rtv;
}

void* dx_create_srv(void* device, void* resource, int fmt, int msaa, int mips, int arraySize, int cube) {
	D3D11_SHADER_RESOURCE_VIEW_DESC desc;
	memset(&desc, 0, sizeof(desc));
	desc.Format = (DXGI_FORMAT)fmt;
	if (cube) {
		desc.ViewDimension = D3D11_SRV_DIMENSION_TEXTURECUBE;
		desc.TextureCube.MipLevels = (UINT)mips;
	} else if (msaa > 1) {
		desc.ViewDimension = D3D11_SRV_DIMENSION_TEXTURE2DMS;
	} else {
		desc.ViewDimension = D3D11_SRV_DIMENSION_TEXTURE2D;
		desc.Texture2D.MipLevels = (UINT)mips;
	}
	ID3D11ShaderResourceView* srv = NULL;
	HRESULT hr = ((ID3D11Device*)device)->lpVtbl->CreateShaderResourceView((ID3D11Device*)device, (ID3D11Resource*)resource, &desc, &srv);
	if (FAILED(hr)) {
		snprintf(dx_err, sizeof(dx_err), "CreateShaderResourceView failed hr=0x%08lx", hr);
		return NULL;
	}
	return srv;
}

void* dx_create_depth(void* device, int w, int h, int msaa) {
	if (msaa < 1) msaa = 1;
	D3D11_TEXTURE2D_DESC desc;
	memset(&desc, 0, sizeof(desc));
	desc.Width = (UINT)w;
	desc.Height = (UINT)h;
	desc.MipLevels = 1;
	desc.ArraySize = 1;
	desc.Format = DXGI_FORMAT_D24_UNORM_S8_UINT;
	desc.SampleDesc.Count = (UINT)msaa;
	desc.SampleDesc.Quality = 0;
	desc.Usage = D3D11_USAGE_DEFAULT;
	desc.BindFlags = D3D11_BIND_DEPTH_STENCIL;
	ID3D11Texture2D* tex = NULL;
	HRESULT hr = ((ID3D11Device*)device)->lpVtbl->CreateTexture2D((ID3D11Device*)device, &desc, NULL, &tex);
	if (FAILED(hr)) {
		snprintf(dx_err, sizeof(dx_err), "CreateTexture2D(depth) failed hr=0x%08lx", hr);
		return NULL;
	}
	D3D11_DEPTH_STENCIL_VIEW_DESC ddesc;
	memset(&ddesc, 0, sizeof(ddesc));
	ddesc.Format = DXGI_FORMAT_D24_UNORM_S8_UINT;
	ddesc.ViewDimension = msaa > 1 ? D3D11_DSV_DIMENSION_TEXTURE2DMS : D3D11_DSV_DIMENSION_TEXTURE2D;
	ID3D11DepthStencilView* dsv = NULL;
	hr = ((ID3D11Device*)device)->lpVtbl->CreateDepthStencilView((ID3D11Device*)device, (ID3D11Resource*)tex, &ddesc, &dsv);
	if (FAILED(hr)) {
		snprintf(dx_err, sizeof(dx_err), "CreateDepthStencilView failed hr=0x%08lx", hr);
		((IUnknown*)tex)->lpVtbl->Release((IUnknown*)tex);
		return NULL;
	}
	((IUnknown*)tex)->lpVtbl->Release((IUnknown*)tex);
	return dsv;
}

void* dx_create_sampler(void* device, int filter, int addrU, int addrV) {
	D3D11_SAMPLER_DESC desc;
	memset(&desc, 0, sizeof(desc));
	desc.Filter = (D3D11_FILTER)filter;
	desc.AddressU = (D3D11_TEXTURE_ADDRESS_MODE)addrU;
	desc.AddressV = (D3D11_TEXTURE_ADDRESS_MODE)addrV;
	desc.AddressW = (D3D11_TEXTURE_ADDRESS_MODE)addrU;
	desc.ComparisonFunc = D3D11_COMPARISON_NEVER;
	desc.MaxLOD = D3D11_FLOAT32_MAX;
	ID3D11SamplerState* s = NULL;
	HRESULT hr = ((ID3D11Device*)device)->lpVtbl->CreateSamplerState((ID3D11Device*)device, &desc, &s);
	if (FAILED(hr)) {
		snprintf(dx_err, sizeof(dx_err), "CreateSamplerState failed hr=0x%08lx", hr);
		return NULL;
	}
	return s;
}

void* dx_create_blend(void* device, int eq, int src, int dst) {
	D3D11_BLEND_DESC desc;
	memset(&desc, 0, sizeof(desc));
	desc.AlphaToCoverageEnable = FALSE;
	desc.IndependentBlendEnable = FALSE;
	for (int i = 0; i < 8; i++) {
		desc.RenderTarget[i].BlendEnable = TRUE;
		desc.RenderTarget[i].SrcBlend = (D3D11_BLEND)src;
		desc.RenderTarget[i].DestBlend = (D3D11_BLEND)dst;
		desc.RenderTarget[i].BlendOp = (D3D11_BLEND_OP)eq;
		desc.RenderTarget[i].SrcBlendAlpha = (D3D11_BLEND)src;
		desc.RenderTarget[i].DestBlendAlpha = (D3D11_BLEND)dst;
		desc.RenderTarget[i].BlendOpAlpha = (D3D11_BLEND_OP)eq;
		desc.RenderTarget[i].RenderTargetWriteMask = D3D11_COLOR_WRITE_ENABLE_ALL;
	}
	ID3D11BlendState* s = NULL;
	HRESULT hr = ((ID3D11Device*)device)->lpVtbl->CreateBlendState((ID3D11Device*)device, &desc, &s);
	if (FAILED(hr)) {
		snprintf(dx_err, sizeof(dx_err), "CreateBlendState failed hr=0x%08lx", hr);
		return NULL;
	}
	return s;
}

void* dx_create_rasterizer(void* device, int scissorEnable) {
	D3D11_RASTERIZER_DESC desc;
	memset(&desc, 0, sizeof(desc));
	desc.FillMode = D3D11_FILL_SOLID;
	desc.CullMode = D3D11_CULL_NONE;
	desc.ScissorEnable = scissorEnable ? TRUE : FALSE;
	desc.DepthClipEnable = TRUE;
	ID3D11RasterizerState* s = NULL;
	HRESULT hr = ((ID3D11Device*)device)->lpVtbl->CreateRasterizerState((ID3D11Device*)device, &desc, &s);
	if (FAILED(hr)) {
		snprintf(dx_err, sizeof(dx_err), "CreateRasterizerState failed hr=0x%08lx", hr);
		return NULL;
	}
	return s;
}

void* dx_create_dsstate(void* device, int depthEnable, int depthWrite) {
	D3D11_DEPTH_STENCIL_DESC desc;
	memset(&desc, 0, sizeof(desc));
	desc.DepthEnable = depthEnable ? TRUE : FALSE;
	desc.DepthWriteMask = depthWrite ? D3D11_DEPTH_WRITE_MASK_ALL : D3D11_DEPTH_WRITE_MASK_ZERO;
	desc.DepthFunc = D3D11_COMPARISON_LESS;
	desc.StencilEnable = FALSE;
	ID3D11DepthStencilState* s = NULL;
	HRESULT hr = ((ID3D11Device*)device)->lpVtbl->CreateDepthStencilState((ID3D11Device*)device, &desc, &s);
	if (FAILED(hr)) {
		snprintf(dx_err, sizeof(dx_err), "CreateDepthStencilState failed hr=0x%08lx", hr);
		return NULL;
	}
	return s;
}

void* dx_create_cb(void* device, int size) {
	D3D11_BUFFER_DESC desc;
	memset(&desc, 0, sizeof(desc));
	desc.ByteWidth = (UINT)size;
	desc.Usage = D3D11_USAGE_DEFAULT;
	desc.BindFlags = D3D11_BIND_CONSTANT_BUFFER;
	ID3D11Buffer* b = NULL;
	HRESULT hr = ((ID3D11Device*)device)->lpVtbl->CreateBuffer((ID3D11Device*)device, &desc, NULL, &b);
	if (FAILED(hr)) {
		snprintf(dx_err, sizeof(dx_err), "CreateBuffer(CB) failed hr=0x%08lx", hr);
		return NULL;
	}
	return b;
}

void* dx_create_vb(void* device, int size) {
	D3D11_BUFFER_DESC desc;
	memset(&desc, 0, sizeof(desc));
	desc.ByteWidth = (UINT)size;
	desc.Usage = D3D11_USAGE_DYNAMIC;
	desc.BindFlags = D3D11_BIND_VERTEX_BUFFER;
	desc.CPUAccessFlags = D3D11_CPU_ACCESS_WRITE;
	ID3D11Buffer* b = NULL;
	HRESULT hr = ((ID3D11Device*)device)->lpVtbl->CreateBuffer((ID3D11Device*)device, &desc, NULL, &b);
	if (FAILED(hr)) {
		snprintf(dx_err, sizeof(dx_err), "CreateBuffer(VB) failed hr=0x%08lx", hr);
		return NULL;
	}
	return b;
}

void* dx_compile(const void* src, int len, int vs) {
	ID3D10Blob* blob = NULL;
	ID3D10Blob* err = NULL;
	HRESULT hr = D3DCompile(src, (SIZE_T)len, "shader", NULL, D3D_COMPILE_STANDARD_FILE_INCLUDE, "main", vs ? "vs_5_0" : "ps_5_0", 0, 0, &blob, &err);
	if (FAILED(hr)) {
		if (err) {
			strncpy(dx_err, (const char*)err->lpVtbl->GetBufferPointer(err), sizeof(dx_err) - 1);
			dx_err[sizeof(dx_err) - 1] = 0;
			err->lpVtbl->Release(err);
		} else {
			snprintf(dx_err, sizeof(dx_err), "D3DCompile failed hr=0x%08lx", hr);
		}
		return NULL;
	}
	if (err) err->lpVtbl->Release(err);
	return blob;
}

const void* dx_blob_ptr(void* blob) { return ((ID3D10Blob*)blob)->lpVtbl->GetBufferPointer((ID3D10Blob*)blob); }
int dx_blob_size(void* blob) { return (int)((ID3D10Blob*)blob)->lpVtbl->GetBufferSize((ID3D10Blob*)blob); }

void* dx_create_vs(void* device, const void* blobPtr, int blobSize) {
	ID3D11VertexShader* s = NULL;
	HRESULT hr = ((ID3D11Device*)device)->lpVtbl->CreateVertexShader((ID3D11Device*)device, blobPtr, (SIZE_T)blobSize, NULL, &s);
	if (FAILED(hr)) {
		snprintf(dx_err, sizeof(dx_err), "CreateVertexShader failed hr=0x%08lx", hr);
		return NULL;
	}
	return s;
}

void* dx_create_ps(void* device, const void* blobPtr, int blobSize) {
	ID3D11PixelShader* s = NULL;
	HRESULT hr = ((ID3D11Device*)device)->lpVtbl->CreatePixelShader((ID3D11Device*)device, blobPtr, (SIZE_T)blobSize, NULL, &s);
	if (FAILED(hr)) {
		snprintf(dx_err, sizeof(dx_err), "CreatePixelShader failed hr=0x%08lx", hr);
		return NULL;
	}
	return s;
}

void* dx_create_il_sprite(void* device, const void* blobPtr, int blobSize) {
	D3D11_INPUT_ELEMENT_DESC desc[2];
	memset(desc, 0, sizeof(desc));
	desc[0].SemanticName = "POSITION";
	desc[0].Format = DXGI_FORMAT_R32G32_FLOAT;
	desc[0].AlignedByteOffset = 0;
	desc[1].SemanticName = "TEXCOORD";
	desc[1].Format = DXGI_FORMAT_R32G32_FLOAT;
	desc[1].AlignedByteOffset = 8;
	ID3D11InputLayout* il = NULL;
	HRESULT hr = ((ID3D11Device*)device)->lpVtbl->CreateInputLayout((ID3D11Device*)device, desc, 2, blobPtr, (SIZE_T)blobSize, &il);
	if (FAILED(hr)) {
		snprintf(dx_err, sizeof(dx_err), "CreateInputLayout failed hr=0x%08lx", hr);
		return NULL;
	}
	return il;
}

void* dx_create_il_pos(void* device, const void* blobPtr, int blobSize) {
	D3D11_INPUT_ELEMENT_DESC desc[1];
	memset(desc, 0, sizeof(desc));
	desc[0].SemanticName = "POSITION";
	desc[0].Format = DXGI_FORMAT_R32G32_FLOAT;
	desc[0].AlignedByteOffset = 0;
	ID3D11InputLayout* il = NULL;
	HRESULT hr = ((ID3D11Device*)device)->lpVtbl->CreateInputLayout((ID3D11Device*)device, desc, 1, blobPtr, (SIZE_T)blobSize, &il);
	if (FAILED(hr)) {
		snprintf(dx_err, sizeof(dx_err), "CreateInputLayout failed hr=0x%08lx", hr);
		return NULL;
	}
	return il;
}

void dx_set_vs(void* ctx, void* vs) { ((ID3D11DeviceContext*)ctx)->lpVtbl->VSSetShader((ID3D11DeviceContext*)ctx, (ID3D11VertexShader*)vs, NULL, 0); }
void dx_set_ps(void* ctx, void* ps) { ((ID3D11DeviceContext*)ctx)->lpVtbl->PSSetShader((ID3D11DeviceContext*)ctx, (ID3D11PixelShader*)ps, NULL, 0); }
void dx_set_il(void* ctx, void* il) { ((ID3D11DeviceContext*)ctx)->lpVtbl->IASetInputLayout((ID3D11DeviceContext*)ctx, (ID3D11InputLayout*)il); }
void dx_set_topology(void* ctx, int topo) { ((ID3D11DeviceContext*)ctx)->lpVtbl->IASetPrimitiveTopology((ID3D11DeviceContext*)ctx, (D3D11_PRIMITIVE_TOPOLOGY)topo); }
void dx_set_vb(void* ctx, void* vb, int stride) {
	ID3D11Buffer* bufs[1] = { (ID3D11Buffer*)vb };
	UINT strides[1] = { (UINT)stride };
	UINT offsets[1] = { 0 };
	((ID3D11DeviceContext*)ctx)->lpVtbl->IASetVertexBuffers((ID3D11DeviceContext*)ctx, 0, 1, bufs, strides, offsets);
}
void dx_set_rt(void* ctx, void* rtv, void* dsv) {
	ID3D11RenderTargetView* rtvs[1] = { (ID3D11RenderTargetView*)rtv };
	((ID3D11DeviceContext*)ctx)->lpVtbl->OMSetRenderTargets((ID3D11DeviceContext*)ctx, 1, rtvs, (ID3D11DepthStencilView*)dsv);
}
void dx_set_viewport(void* ctx, int x, int y, int w, int h) {
	D3D11_VIEWPORT vp;
	vp.TopLeftX = (FLOAT)x;
	vp.TopLeftY = (FLOAT)y;
	vp.Width = (FLOAT)w;
	vp.Height = (FLOAT)h;
	vp.MinDepth = 0.0f;
	vp.MaxDepth = 1.0f;
	((ID3D11DeviceContext*)ctx)->lpVtbl->RSSetViewports((ID3D11DeviceContext*)ctx, 1, &vp);
}
void dx_set_scissor(void* ctx, int x, int y, int w, int h, int enabled) {
	D3D11_RECT r;
	r.left = x; r.top = y; r.right = x + w; r.bottom = y + h;
	((ID3D11DeviceContext*)ctx)->lpVtbl->RSSetScissorRects((ID3D11DeviceContext*)ctx, enabled ? 1 : 0, &r);
}
void dx_set_blend(void* ctx, void* blend) {
	FLOAT f[4] = { 0, 0, 0, 0 };
	((ID3D11DeviceContext*)ctx)->lpVtbl->OMSetBlendState((ID3D11DeviceContext*)ctx, (ID3D11BlendState*)blend, f, 0xffffffff);
}
void dx_set_dsstate(void* ctx, void* ds) { ((ID3D11DeviceContext*)ctx)->lpVtbl->OMSetDepthStencilState((ID3D11DeviceContext*)ctx, (ID3D11DepthStencilState*)ds, 0); }
void dx_set_rasterizer(void* ctx, void* rs) { ((ID3D11DeviceContext*)ctx)->lpVtbl->RSSetState((ID3D11DeviceContext*)ctx, (ID3D11RasterizerState*)rs); }
void dx_set_cb_vs(void* ctx, int slot, void* cb) {
	ID3D11Buffer* bufs[1] = { (ID3D11Buffer*)cb };
	((ID3D11DeviceContext*)ctx)->lpVtbl->VSSetConstantBuffers((ID3D11DeviceContext*)ctx, (UINT)slot, 1, bufs);
}
void dx_set_cb_ps(void* ctx, int slot, void* cb) {
	ID3D11Buffer* bufs[1] = { (ID3D11Buffer*)cb };
	((ID3D11DeviceContext*)ctx)->lpVtbl->PSSetConstantBuffers((ID3D11DeviceContext*)ctx, (UINT)slot, 1, bufs);
}
void dx_set_srv_ps(void* ctx, int slot, void* srv) {
	ID3D11ShaderResourceView* srvs[1] = { (ID3D11ShaderResourceView*)srv };
	((ID3D11DeviceContext*)ctx)->lpVtbl->PSSetShaderResources((ID3D11DeviceContext*)ctx, (UINT)slot, 1, srvs);
}
void dx_set_sampler_ps(void* ctx, int slot, void* s) {
	ID3D11SamplerState* samps[1] = { (ID3D11SamplerState*)s };
	((ID3D11DeviceContext*)ctx)->lpVtbl->PSSetSamplers((ID3D11DeviceContext*)ctx, (UINT)slot, 1, samps);
}
void dx_update_cb(void* ctx, void* cb, const void* data, int size) {
	((ID3D11DeviceContext*)ctx)->lpVtbl->UpdateSubresource((ID3D11DeviceContext*)ctx, (ID3D11Resource*)cb, 0, NULL, data, (UINT)size, 0);
}
void dx_update_sub(void* ctx, void* res, const void* data, int rowPitch, int x, int y, int w, int h) {
	D3D11_BOX box;
	box.left = (UINT)x; box.top = (UINT)y; box.front = 0;
	box.right = (UINT)(x + w); box.bottom = (UINT)(y + h); box.back = 1;
	((ID3D11DeviceContext*)ctx)->lpVtbl->UpdateSubresource((ID3D11DeviceContext*)ctx, (ID3D11Resource*)res, 0, &box, data, (UINT)rowPitch, 0);
}
void* dx_map_vb(void* ctx, void* vb) {
	D3D11_MAPPED_SUBRESOURCE mapped;
	HRESULT hr = ((ID3D11DeviceContext*)ctx)->lpVtbl->Map((ID3D11DeviceContext*)ctx, (ID3D11Resource*)vb, 0, D3D11_MAP_WRITE_DISCARD, 0, &mapped);
	if (FAILED(hr)) {
		snprintf(dx_err, sizeof(dx_err), "Map(VB) failed hr=0x%08lx", hr);
		return NULL;
	}
	return mapped.pData;
}
void dx_unmap_vb(void* ctx, void* vb) {
	((ID3D11DeviceContext*)ctx)->lpVtbl->Unmap((ID3D11DeviceContext*)ctx, (ID3D11Resource*)vb, 0);
}
void dx_draw(void* ctx, int count, int start) {
	((ID3D11DeviceContext*)ctx)->lpVtbl->Draw((ID3D11DeviceContext*)ctx, (UINT)count, (UINT)start);
}
void dx_draw_indexed(void* ctx, int count, int offsetBytes) {
	((ID3D11DeviceContext*)ctx)->lpVtbl->DrawIndexed((ID3D11DeviceContext*)ctx, (UINT)count, (UINT)(offsetBytes / 4), 0);
}
void dx_clear_rt(void* ctx, void* rtv, float r, float g, float b, float a) {
	FLOAT c[4] = { r, g, b, a };
	((ID3D11DeviceContext*)ctx)->lpVtbl->ClearRenderTargetView((ID3D11DeviceContext*)ctx, (ID3D11RenderTargetView*)rtv, c);
}
void dx_clear_depth(void* ctx, void* dsv) {
	((ID3D11DeviceContext*)ctx)->lpVtbl->ClearDepthStencilView((ID3D11DeviceContext*)ctx, (ID3D11DepthStencilView*)dsv, D3D11_CLEAR_DEPTH | D3D11_CLEAR_STENCIL, 1.0f, 0);
}
void dx_resolve(void* ctx, void* dst, void* src, int fmt) {
	((ID3D11DeviceContext*)ctx)->lpVtbl->ResolveSubresource((ID3D11DeviceContext*)ctx, (ID3D11Resource*)dst, 0, (ID3D11Resource*)src, 0, (DXGI_FORMAT)fmt);
}
void dx_copy(void* ctx, void* dst, void* src) {
	((ID3D11DeviceContext*)ctx)->lpVtbl->CopyResource((ID3D11DeviceContext*)ctx, (ID3D11Resource*)dst, (ID3D11Resource*)src);
}
void dx_copy_region(void* ctx, void* dst, void* src, int w, int h) {
	D3D11_BOX box;
	box.left = 0; box.top = 0; box.front = 0;
	box.right = (UINT)w; box.bottom = (UINT)h; box.back = 1;
	((ID3D11DeviceContext*)ctx)->lpVtbl->CopySubresourceRegion((ID3D11DeviceContext*)ctx, (ID3D11Resource*)dst, 0, 0, 0, 0, (ID3D11Resource*)src, 0, &box);
}
void dx_generate_mips(void* ctx, void* srv) {
	((ID3D11DeviceContext*)ctx)->lpVtbl->GenerateMips((ID3D11DeviceContext*)ctx, (ID3D11ShaderResourceView*)srv);
}
void dx_flush(void* ctx) {
	((ID3D11DeviceContext*)ctx)->lpVtbl->Flush((ID3D11DeviceContext*)ctx);
}
void* dx_map_read(void* ctx, void* res) {
	D3D11_MAPPED_SUBRESOURCE mapped;
	HRESULT hr = ((ID3D11DeviceContext*)ctx)->lpVtbl->Map((ID3D11DeviceContext*)ctx, (ID3D11Resource*)res, 0, D3D11_MAP_READ, 0, &mapped);
	if (FAILED(hr)) {
		snprintf(dx_err, sizeof(dx_err), "Map(READ) failed hr=0x%08lx", hr);
		return NULL;
	}
	return mapped.pData;
}
void dx_unmap_read(void* ctx, void* res) {
	((ID3D11DeviceContext*)ctx)->lpVtbl->Unmap((ID3D11DeviceContext*)ctx, (ID3D11Resource*)res, 0);
}
void dx_release(void* obj) {
	if (obj) ((IUnknown*)obj)->lpVtbl->Release((IUnknown*)obj);
}
void dx_device_name(void* device, char* buf, int maxlen) {
	buf[0] = 0;
	IDXGIDevice* dxgiDevice = NULL;
	HRESULT hr = ((ID3D11Device*)device)->lpVtbl->QueryInterface((ID3D11Device*)device, &IID_IDXGIDevice, (void**)&dxgiDevice);
	if (FAILED(hr) || !dxgiDevice) return;
	IDXGIAdapter* adapter = NULL;
	hr = dxgiDevice->lpVtbl->GetAdapter(dxgiDevice, &adapter);
	dxgiDevice->lpVtbl->Release(dxgiDevice);
	if (FAILED(hr) || !adapter) return;
	DXGI_ADAPTER_DESC desc;
	if (SUCCEEDED(adapter->lpVtbl->GetDesc(adapter, &desc))) {
		WideCharToMultiByte(CP_UTF8, 0, desc.Description, -1, buf, maxlen - 1, NULL, NULL);
	}
	adapter->lpVtbl->Release(adapter);
}
*/
import "C"

//go:embed shaders/sprite_vs.hlsl
var dxSpriteVS string

//go:embed shaders/sprite_ps.hlsl
var dxSpritePS string

//go:embed shaders/font_vs.hlsl
var dxFontVS string

//go:embed shaders/font_ps.hlsl
var dxFontPS string

//go:embed shaders/ident_vs.hlsl
var dxIdentVS string

//go:embed shaders/ident_ps.hlsl
var dxIdentPS string

const (
	dxFmtRGBA8   = 28
	dxFmtR8      = 61
	dxFmtRGB32F  = 6
	dxFmtRGBA32F = 2
	dxFmtRGBA16F = 10

	dxBindSRV   = 0x8
	dxBindRTV   = 0x20
	dxBindDepth = 0x40

	dxUsageDefault = 0
	dxUsageDynamic = 1
	dxUsageStaging = 3

	dxCPUWrite = 0x10000
	dxCPURead  = 0x20000

	dxTopoPoints    = 1
	dxTopoLine      = 2
	dxTopoLineStrip = 3
	dxTopoList      = 4
	dxTopoStrip     = 5
	dxTopoFan       = 6

	dxBlendOpAdd    = 1
	dxBlendOpRevSub = 4

	dxBlendZero        = 1
	dxBlendOne         = 2
	dxBlendSrcAlpha    = 5
	dxBlendInvSrcAlpha = 6
	dxBlendDstColor    = 9
	dxBlendInvDstColor = 10

	dxAddrWrap   = 1
	dxAddrMirror = 2
	dxAddrClamp  = 3

	dxFilterMinMagMipPoint     = 0
	dxFilterMinMagPointMipLine = 0x1
	dxFilterMinPointMagLineMP  = 0x4
	dxFilterMinPointMagLineML  = 0x5
	dxFilterMinLineMagMipPoint = 0x10
	dxFilterMinLineMagPointML  = 0x11
	dxFilterMinMagLineMipPoint = 0x14
	dxFilterMinMagMipLine      = 0x15

	dxMaxTextureSlots = 8
)

type Texture_DX struct {
	width, height, depth int32
	filter               bool
	resource             unsafe.Pointer
	srv                  unsafe.Pointer
	sampler              unsafe.Pointer
	serial               uint64
	offsetX, offsetY     int32
	slotIndex            int32
	palSlot              bool
	atlasSize            int32
	isCube               bool
	mips                 int
}

func (t *Texture_DX) bpp() int {
	if t.depth == 24 {
		return 3
	}
	return int(Max(int32(t.depth/8), 1))
}

func (t *Texture_DX) setFilterParams(r *Renderer_DX) {
	if t.sampler != nil {
		return
	}
	if t.filter {
		t.sampler = r.linearClamp
	} else {
		t.sampler = r.pointClamp
	}
}

func (t *Texture_DX) SetData(data []byte) {
	r := gfx.(*Renderer_DX)
	if t.palSlot {
		if len(data) == 0 {
			return
		}
		C.dx_update_sub(r.ctx, t.resource, unsafe.Pointer(&data[0]), 1024, C.int(t.offsetX), C.int(t.offsetY), 256, 1)
		return
	}
	rowPitch := int(t.width) * t.bpp()
	data32 := convertRows(data, int(t.width), int(t.height), int(t.depth))
	if data32 != nil {
		rowPitch = int(t.width) * 4
	}
	C.dx_update_sub(r.ctx, t.resource, dataPtr(data, data32), C.int(rowPitch), 0, 0, C.int(t.width), C.int(t.height))
	t.setFilterParams(r)
}

func (t *Texture_DX) SetSubData(data []byte, x, y, width, height, stride int32) {
	r := gfx.(*Renderer_DX)
	rowPitch := stride
	bpp := int32(t.bpp())
	if rowPitch <= 0 || rowPitch != width*bpp {
		rowPitch = width * bpp
	}
	data32 := convertRows(data, int(width), int(height), int(t.depth))
	if data32 != nil {
		rowPitch = width * 4
	}
	C.dx_update_sub(r.ctx, t.resource, dataPtr(data, data32), C.int(rowPitch), C.int(x), C.int(y), C.int(width), C.int(height))
	t.setFilterParams(r)
}

func (t *Texture_DX) SetDataG(data []byte, mag, min, ws, wt TextureSamplingParam) {
	r := gfx.(*Renderer_DX)
	if t.sampler == nil {
		t.sampler = r.getSampler(mag, min, ws, wt)
	}
	rowPitch := int(t.width) * t.bpp()
	data32 := convertRows(data, int(t.width), int(t.height), int(t.depth))
	if data32 != nil {
		rowPitch = int(t.width) * 4
	}
	C.dx_update_sub(r.ctx, t.resource, dataPtr(data, data32), C.int(rowPitch), 0, 0, C.int(t.width), C.int(t.height))
	if t.mips > 1 && t.srv != nil {
		C.dx_generate_mips(r.ctx, t.srv)
	}
}

func (t *Texture_DX) SetPixelData(data []float32) {
	r := gfx.(*Renderer_DX)
	var p unsafe.Pointer
	if len(data) > 0 {
		p = unsafe.Pointer(&data[0])
	}
	C.dx_update_sub(r.ctx, t.resource, p, C.int(t.width*4), 0, 0, C.int(t.width), C.int(t.height))
}

func (t *Texture_DX) IsValid() bool {
	return t.width != 0 && t.height != 0 && t.srv != nil
}

func (t *Texture_DX) GetWidth() int32  { return t.width }
func (t *Texture_DX) GetHeight() int32 { return t.height }

func (t *Texture_DX) GetPalUV() [4]float32 {
	if t.palSlot {
		return [4]float32{
			(float32(t.offsetX) + 0.5) / float32(t.atlasSize),
			(float32(t.offsetY) + 0.5) / float32(t.atlasSize),
			float32(256) / float32(t.atlasSize),
			float32(1) / float32(t.atlasSize),
		}
	}
	return [4]float32{0, 0.5, 1, 0}
}

func (t *Texture_DX) CopyData(src *Texture) {
	r := gfx.(*Renderer_DX)
	s := (*src).(*Texture_DX)
	if s.resource == nil || t.resource == nil {
		return
	}
	w := Min(t.width, s.width)
	h := Min(t.height, s.height)
	C.dx_copy_region(r.ctx, t.resource, s.resource, C.int(w), C.int(h))
}

func (t *Texture_DX) Release() {
	if t.resource == nil && t.srv == nil {
		return
	}
	if t.palSlot {
		t.resource = nil
		t.srv = nil
		return
	}
	C.dx_release(t.srv)
	C.dx_release(t.resource)
	t.srv = nil
	t.resource = nil
}

func (t *Texture_DX) GetSerial() uint64 { return t.serial }

func dxMapFormat(depth int32) int {
	switch depth {
	case 8:
		return dxFmtR8
	case 96:
		return dxFmtRGB32F
	case 128:
		return dxFmtRGBA32F
	default:
		return dxFmtRGBA8
	}
}

func dataPtr(data []byte, converted []byte) unsafe.Pointer {
	if converted != nil {
		return unsafe.Pointer(&converted[0])
	}
	if len(data) > 0 {
		return unsafe.Pointer(&data[0])
	}
	return nil
}

func convertRows(data []byte, w, h, depth int) []byte {
	if depth != 24 || len(data) == 0 {
		return nil
	}
	out := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		src := data[y*w*3:]
		dst := out[y*w*4:]
		for x := 0; x < w; x++ {
			dst[x*4] = src[x*3]
			dst[x*4+1] = src[x*3+1]
			dst[x*4+2] = src[x*3+2]
			dst[x*4+3] = 255
		}
	}
	return out
}

type dxUniforms struct {
	modelview, projection [16]float32
	x1x2x4x3              [4]float32
	tint                  [4]float32
	palUV                 [4]float32
	add                   [4]float32
	mult                  [4]float32
	alphaGrayHue          [4]float32
	maskFlatRgbaTrapez    [4]int32
	negTime               [4]float32
	iResolution           [4]float32
	p                     [16][4]float32
}

type dxFontUniforms struct {
	textColor  [4]float32
	resolution [4]float32
	palAddGray [4]float32
	palMulHue  [4]float32
	palNeg     [4]float32
}

type dxPostUniforms struct {
	textureSize [4]float32
	currentTime [4]float32
}

type dxPostShader struct {
	vs, ps unsafe.Pointer
}

type dxCustomShader struct {
	ps            unsafe.Pointer
	needsGrabPass bool
}

type dxState struct {
	vs, ps, il unsafe.Pointer
	topo       int
	rtv, dsv   unsafe.Pointer
	vp         [4]int32
	blend      unsafe.Pointer
	ds         unsafe.Pointer
	rs         unsafe.Pointer
	scissor    [4]int32
	scissorOn  bool
	srv        [dxMaxTextureSlots]unsafe.Pointer
	sampler    [dxMaxTextureSlots]unsafe.Pointer
	cbVS, cbPS unsafe.Pointer
	vb         unsafe.Pointer
	vbStride   int
}

type Renderer_DX struct {
	device    unsafe.Pointer
	ctx       unsafe.Pointer
	swapchain unsafe.Pointer
	hwnd      unsafe.Pointer

	width, height int32
	winW, winH    int32

	backbuffer    unsafe.Pointer
	backbufferRTV unsafe.Pointer

	rtMain    unsafe.Pointer
	rtMainSRV unsafe.Pointer
	rtMainRTV unsafe.Pointer
	rtMsaa    unsafe.Pointer
	rtMsaaRTV unsafe.Pointer
	depthDSV  unsafe.Pointer
	ppTex     [2]unsafe.Pointer
	ppSRV     [2]unsafe.Pointer
	ppRTV     [2]unsafe.Pointer
	grabTex   unsafe.Pointer
	grabSRV   unsafe.Pointer

	grabTexture *Texture_DX

	palAtlas      *Texture_DX
	palFreeSlots  *list.List
	palLock       sync.Mutex
	oldPalAtlases []*Texture_DX
	palAtlasSize  int32

	vsSprite unsafe.Pointer
	psSprite unsafe.Pointer
	vsIdent  unsafe.Pointer
	psIdent  unsafe.Pointer
	ilSprite unsafe.Pointer
	ilPost   unsafe.Pointer
	vb       unsafe.Pointer
	vbSize   int
	postVB   unsafe.Pointer

	vertexScratch []byte

	cb      unsafe.Pointer
	cbDirty bool
	cbFont  unsafe.Pointer
	cbPost  unsafe.Pointer

	uniforms dxUniforms

	pointClamp   unsafe.Pointer
	linearClamp  unsafe.Pointer
	samplerCache map[uint32]unsafe.Pointer
	blendCache   map[[3]int]unsafe.Pointer
	rsDefault    unsafe.Pointer
	rsScissor    unsafe.Pointer
	dsOff        unsafe.Pointer
	dsOn         unsafe.Pointer

	customShaders   map[uint32]*dxCustomShader
	customShaderMap map[string]uint32
	nextShaderID    uint32
	currentCustom   uint32
	postShaders     []dxPostShader

	state dxState

	vsyncInterval int
	msaa          int32
}

func (r *Renderer_DX) GetName() string   { return "Direct3D 11" }
func (r *Renderer_DX) DebugInfo() string { return "" }

func (r *Renderer_DX) Init() {
	if sys.msaa != 0 && sys.msaa != 2 && sys.msaa != 4 && sys.msaa != 8 {
		LogMessage("[Direct3D 11] Unsupported MSAA level %d, forcing off", sys.msaa)
		sys.msaa = 0
	}
	r.msaa = sys.msaa
	r.width, r.height = sys.scrrect[2], sys.scrrect[3]

	info, err := sys.window.GetWMInfo()
	if err != nil {
		panic(fmt.Sprintf("Direct3D 11: SDL_GetWindowWMInfo failed: %v", err))
	}
	f, _ := reflect.TypeOf(info).Elem().FieldByName("dummy")
	off := f.Offset
	r.hwnd = *(*unsafe.Pointer)(unsafe.Pointer(uintptr(unsafe.Pointer(info)) + off))

	r.device = C.dx_create_device(C.int(Btoi(sys.cfg.Video.RendererDebugMode)))
	if r.device == nil {
		panic("Direct3D 11: " + C.GoString(C.dx_last_error()))
	}
	r.ctx = C.dx_get_context(r.device)
	if r.ctx == nil {
		panic("Direct3D 11: failed to get immediate context")
	}
	w, h := sys.window.GetSize()
	r.winW, r.winH = int32(w), int32(h)

	factory := C.dx_create_factory()
	if factory == nil {
		panic("Direct3D 11: " + C.GoString(C.dx_last_error()))
	}
	r.swapchain = C.dx_create_swapchain(factory, r.device, r.hwnd, C.int(r.winW), C.int(r.winH))
	C.dx_release(factory)
	if r.swapchain == nil {
		panic("Direct3D 11: " + C.GoString(C.dx_last_error()))
	}

	var nameBuf [256]byte
	C.dx_device_name(r.device, (*C.char)(unsafe.Pointer(&nameBuf[0])), 256)
	LogMessage("[Direct3D 11] Device: %s (debug layer: %v)", C.GoString((*C.char)(unsafe.Pointer(&nameBuf[0]))), C.dx_debug_layer() != 0)

	r.createBackbuffer()

	msaa := 0
	if r.msaa > 0 {
		msaa = int(r.msaa)
	}

	r.rtMain = C.dx_create_texture(r.device, C.int(r.width), C.int(r.height), dxFmtRGBA8, dxBindSRV|dxBindRTV, 1, 1, 1, 0, dxUsageDefault, 0)
	chkRes(r.rtMain)
	r.rtMainSRV = C.dx_create_srv(r.device, r.rtMain, dxFmtRGBA8, 1, 1, 1, 0)
	chkRes(r.rtMainSRV)
	r.rtMainRTV = C.dx_create_rtv(r.device, r.rtMain, 1)
	chkRes(r.rtMainRTV)
	if msaa > 0 {
		r.rtMsaa = C.dx_create_texture(r.device, C.int(r.width), C.int(r.height), dxFmtRGBA8, dxBindRTV, C.int(msaa), 1, 1, 0, dxUsageDefault, 0)
		chkRes(r.rtMsaa)
		r.rtMsaaRTV = C.dx_create_rtv(r.device, r.rtMsaa, C.int(msaa))
		chkRes(r.rtMsaaRTV)
	}
	r.depthDSV = C.dx_create_depth(r.device, C.int(r.width), C.int(r.height), C.int(msaa))
	chkRes(r.depthDSV)

	for i := 0; i < 2; i++ {
		r.ppTex[i] = C.dx_create_texture(r.device, C.int(r.width), C.int(r.height), dxFmtRGBA16F, dxBindSRV|dxBindRTV, 1, 1, 1, 0, dxUsageDefault, 0)
		chkRes(r.ppTex[i])
		r.ppSRV[i] = C.dx_create_srv(r.device, r.ppTex[i], dxFmtRGBA16F, 1, 1, 1, 0)
		chkRes(r.ppSRV[i])
		r.ppRTV[i] = C.dx_create_rtv(r.device, r.ppTex[i], 1)
		chkRes(r.ppRTV[i])
	}

	r.grabTexture = r.newTextureInternal(r.width, r.height, 32, true)
	r.grabTex = r.grabTexture.resource
	r.grabSRV = r.grabTexture.srv

	r.pointClamp = C.dx_create_sampler(r.device, dxFilterMinMagMipPoint, dxAddrClamp, dxAddrClamp)
	chkRes(r.pointClamp)
	r.linearClamp = C.dx_create_sampler(r.device, dxFilterMinMagMipLine, dxAddrClamp, dxAddrClamp)
	chkRes(r.linearClamp)
	r.samplerCache = make(map[uint32]unsafe.Pointer)
	r.blendCache = make(map[[3]int]unsafe.Pointer)
	r.rsDefault = C.dx_create_rasterizer(r.device, 0)
	chkRes(r.rsDefault)
	r.rsScissor = C.dx_create_rasterizer(r.device, 1)
	chkRes(r.rsScissor)
	r.dsOff = C.dx_create_dsstate(r.device, 0, 0)
	chkRes(r.dsOff)
	r.dsOn = C.dx_create_dsstate(r.device, 1, 1)
	chkRes(r.dsOn)

	var vsSpriteBlob, vsIdentBlob unsafe.Pointer
	r.vsSprite, vsSpriteBlob = r.compileShader(dxSpriteVS, true)
	r.psSprite, _ = r.compileShader(dxSpritePS, false)
	r.vsIdent, vsIdentBlob = r.compileShader(dxIdentVS, true)
	r.psIdent, _ = r.compileShader(dxIdentPS, false)

	r.ilSprite = C.dx_create_il_sprite(r.device, C.dx_blob_ptr(vsSpriteBlob), C.int(C.dx_blob_size(vsSpriteBlob)))
	chkRes(r.ilSprite)
	r.ilPost = C.dx_create_il_pos(r.device, C.dx_blob_ptr(vsIdentBlob), C.int(C.dx_blob_size(vsIdentBlob)))
	chkRes(r.ilPost)
	C.dx_release(vsSpriteBlob)
	C.dx_release(vsIdentBlob)

	r.vb = C.dx_create_vb(r.device, 65536)
	chkRes(r.vb)
	r.vbSize = 65536

	postData := []float32{-1, -1, 1, -1, -1, 1, 1, 1}
	postBytes := make([]byte, 32)
	for i, v := range postData {
		binary.LittleEndian.PutUint32(postBytes[i*4:], math.Float32bits(v))
	}
	r.postVB = C.dx_create_vb(r.device, 32)
	chkRes(r.postVB)
	p := C.dx_map_vb(r.ctx, r.postVB)
	chkRes(p)
	copy(unsafe.Slice((*byte)(p), 32), postBytes)
	C.dx_unmap_vb(r.ctx, r.postVB)

	r.cb = C.dx_create_cb(r.device, C.int(unsafe.Sizeof(dxUniforms{})))
	chkRes(r.cb)
	r.cbFont = C.dx_create_cb(r.device, C.int(unsafe.Sizeof(dxFontUniforms{})))
	chkRes(r.cbFont)
	r.cbPost = C.dx_create_cb(r.device, C.int(unsafe.Sizeof(dxPostUniforms{})))
	chkRes(r.cbPost)

	r.customShaders = make(map[uint32]*dxCustomShader)
	r.customShaderMap = make(map[string]uint32)
	r.nextShaderID = 1

	r.createPostShaders()
	r.createPalAtlas()

	r.vsyncInterval = 1
}

func (r *Renderer_DX) createPostShaders() {
	ext := sys.externalShaders
	r.postShaders = make([]dxPostShader, 0, 1)
	if len(ext) > 0 {
		n := len(ext[0])
		cap := n + 1
		r.postShaders = make([]dxPostShader, 0, cap)
		for i := 0; i < n; i++ {
			vsBytes := ext[0][i]
			psBytes := ext[1][i]
			var vs, ps unsafe.Pointer
			if len(vsBytes) > 0 {
				vs = C.dx_create_vs(r.device, unsafe.Pointer(&vsBytes[0]), C.int(len(vsBytes)))
			}
			if len(psBytes) > 0 {
				ps = C.dx_create_ps(r.device, unsafe.Pointer(&psBytes[0]), C.int(len(psBytes)))
			}
			if vs != nil && ps != nil {
				r.postShaders = append(r.postShaders, dxPostShader{vs: vs, ps: ps})
			} else {
				LogMessage("[Direct3D 11] External shader %d failed to load", i)
			}
		}
	}
	r.postShaders = append(r.postShaders, dxPostShader{vs: r.vsIdent, ps: r.psIdent})
}

func chkRes(p unsafe.Pointer) {
	if p == nil {
		panic("Direct3D 11: " + C.GoString(C.dx_last_error()))
	}
}

func (r *Renderer_DX) compileShader(source string, vs bool) (unsafe.Pointer, unsafe.Pointer) {
	data := unsafe.StringData(source)
	blob := C.dx_compile(unsafe.Pointer(data), C.int(len(source)), C.int(Btoi(vs)))
	if blob == nil {
		panic("Direct3D 11: " + C.GoString(C.dx_last_error()))
	}
	var sh unsafe.Pointer
	if vs {
		sh = C.dx_create_vs(r.device, C.dx_blob_ptr(blob), C.int(C.dx_blob_size(blob)))
	} else {
		sh = C.dx_create_ps(r.device, C.dx_blob_ptr(blob), C.int(C.dx_blob_size(blob)))
	}
	chkRes(sh)
	return sh, blob
}

func (r *Renderer_DX) createBackbuffer() {
	if r.backbuffer != nil {
		C.dx_release(r.backbufferRTV)
		C.dx_release(r.backbuffer)
		r.backbuffer, r.backbufferRTV = nil, nil
	}
	r.backbuffer = C.dx_get_backbuffer(r.swapchain)
	chkRes(r.backbuffer)
	r.backbufferRTV = C.dx_create_rtv(r.device, r.backbuffer, 1)
	chkRes(r.backbufferRTV)
}

func (r *Renderer_DX) createPalAtlas() {
	r.palAtlasSize = Max(PalAtlasSize, 256)
	atlas := r.newTextureInternal(r.palAtlasSize, r.palAtlasSize, 32, false)
	atlas.SetData(make([]byte, r.palAtlasSize*r.palAtlasSize*4))
	r.palAtlas = atlas
	r.palFreeSlots = list.New()
	slotsPerRow := r.palAtlasSize / 256
	for i := 0; i < int(r.palAtlasSize)*int(slotsPerRow); i++ {
		r.palFreeSlots.PushBack(int32(i))
	}
	memPalSlotSetTotal(int64(r.palAtlasSize))
}

func (r *Renderer_DX) autoResizeAtlas() {
	if r.palAtlasSize >= 4096 {
		return
	}
	newSize := Min(r.palAtlasSize*2, 4096)
	atlas := r.newTextureInternal(newSize, newSize, 32, false)
	atlas.SetData(make([]byte, newSize*newSize*4))
	r.oldPalAtlases = append(r.oldPalAtlases, r.palAtlas)
	r.palAtlas = atlas
	r.palAtlasSize = newSize
	slotsPerRow := newSize / 256
	for i := 0; i < int(newSize)*int(slotsPerRow); i++ {
		r.palFreeSlots.PushBack(int32(i))
	}
	sys.cfg.SetValueUpdate("Video.PaletteAtlasSize", newSize)
	memPalSlotSetTotal(int64(newSize))
}

func (r *Renderer_DX) newTextureInternal(width, height, depth int32, filter bool) *Texture_DX {
	format := dxMapFormat(Max(depth, 8))
	t := &Texture_DX{
		width:  width,
		height: height,
		depth:  depth,
		filter: filter,
		serial: textureSerialNumber,
	}
	textureSerialNumber++
	t.resource = C.dx_create_texture(r.device, C.int(width), C.int(height), C.int(format), dxBindSRV, 1, 1, 1, 0, dxUsageDefault, 0)
	chkRes(t.resource)
	t.srv = C.dx_create_srv(r.device, t.resource, C.int(format), 1, 1, 1, 0)
	chkRes(t.srv)
	t.setFilterParams(r)
	runtime.SetFinalizer(t, func(t *Texture_DX) { t.Release() })
	return t
}

func (r *Renderer_DX) getSampler(mag, min, ws, wt TextureSamplingParam) unsafe.Pointer {
	key := uint32(min)<<12 | uint32(mag)<<8 | uint32(ws)<<4 | uint32(wt)
	if s, ok := r.samplerCache[key]; ok {
		return s
	}
	minLinear := min == TextureSamplingFilterLinear || min == TextureSamplingFilterLinearMipMapNearest || min == TextureSamplingFilterLinearMipMapLinear
	mipLinear := min == TextureSamplingFilterNearestMipMapLinear || min == TextureSamplingFilterLinearMipMapLinear
	magLinear := mag != TextureSamplingFilterNearest
	idx := Btoi(minLinear)<<2 | Btoi(magLinear)<<1 | Btoi(mipLinear)
	filter := [8]int{
		dxFilterMinMagMipPoint,
		dxFilterMinMagPointMipLine,
		dxFilterMinPointMagLineMP,
		dxFilterMinPointMagLineML,
		dxFilterMinLineMagMipPoint,
		dxFilterMinLineMagPointML,
		dxFilterMinMagLineMipPoint,
		dxFilterMinMagMipLine,
	}[idx]
	addr := dxAddrClamp
	switch {
	case ws == TextureSamplingWrapMirroredRepeat:
		addr = dxAddrMirror
	case ws == TextureSamplingWrapRepeat:
		addr = dxAddrWrap
	}
	s := C.dx_create_sampler(r.device, C.int(filter), C.int(addr), C.int(addr))
	chkRes(s)
	r.samplerCache[key] = s
	return s
}

func (r *Renderer_DX) Close() {
	C.dx_release(r.psSprite)
	C.dx_release(r.vsSprite)
	C.dx_release(r.psIdent)
	C.dx_release(r.vsIdent)
	C.dx_release(r.ilSprite)
	C.dx_release(r.ilPost)
	C.dx_release(r.vb)
	C.dx_release(r.postVB)
	C.dx_release(r.cb)
	C.dx_release(r.cbFont)
	C.dx_release(r.cbPost)
	C.dx_release(r.rtMain)
	C.dx_release(r.rtMainSRV)
	C.dx_release(r.rtMainRTV)
	C.dx_release(r.rtMsaa)
	C.dx_release(r.rtMsaaRTV)
	C.dx_release(r.depthDSV)
	C.dx_release(r.backbufferRTV)
	C.dx_release(r.backbuffer)
	C.dx_release(r.swapchain)
	C.dx_release(r.ctx)
	C.dx_release(r.device)
}

func (r *Renderer_DX) BeginFrame(clearColor bool) {
	r.checkResize()
	target := r.rtMainRTV
	if r.rtMsaaRTV != nil {
		target = r.rtMsaaRTV
	}
	r.bindRT(target, r.depthDSV)
	r.bindViewport(0, 0, r.width, r.height)
	C.dx_clear_depth(r.ctx, r.depthDSV)
	if clearColor {
		C.dx_clear_rt(r.ctx, target, 0, 0, 0, 0)
	}
	drawCallStats.reset()
	lastRenderParams = nil
	resetSpriteQueue()
}

func (r *Renderer_DX) checkResize() {
	w, h := sys.window.GetSize()
	if w <= 0 || h <= 0 {
		return
	}
	if int32(w) == r.winW && int32(h) == r.winH {
		return
	}
	if C.dx_resize(r.swapchain, C.int(w), C.int(h)) != 0 {
		LogMessage("[Direct3D 11] ResizeBuffers failed: %s", C.GoString(C.dx_last_error()))
		return
	}
	r.winW, r.winH = int32(w), int32(h)
	r.createBackbuffer()
}

func (r *Renderer_DX) EndFrame() {
	flushSpriteQueue()
	if r.rtMsaaRTV != nil {
		C.dx_resolve(r.ctx, r.rtMain, r.rtMsaa, dxFmtRGBA8)
	}

	r.bindBlend(nil)
	r.bindDS(r.dsOff)
	r.DisableScissor()

	scaleMode := dxFilterMinMagMipPoint
	if sys.cfg.Video.WindowScaleMode {
		scaleMode = dxFilterMinMagMipLine
	}

	n := len(r.postShaders)
	input := r.rtMainSRV
	for i, sh := range r.postShaders {
		if i > 0 {
			input = r.ppSRV[(i-1)%2]
		}
		outRTV := r.ppRTV[i%2]
		outW, outH := r.width, r.height
		if i == n-1 {
			outRTV = r.backbufferRTV
			x, y, w, h := sys.window.GetScaledViewportSize()
			outW, outH = w, h
			r.bindViewport(x, y, w, h)
		} else {
			r.bindViewport(0, 0, r.width, r.height)
		}
		r.bindRT(outRTV, nil)
		C.dx_clear_rt(r.ctx, outRTV, 0, 0, 0, 0)
		r.bindShaders(sh.vs, sh.ps)
		r.bindIL(r.ilPost)
		r.bindTopology(dxTopoStrip)
		r.bindVB(r.postVB, 8)
		u := dxPostUniforms{}
		u.textureSize[0] = float32(outW)
		u.textureSize[1] = float32(outH)
		u.currentTime[0] = float32(sdl.GetPerformanceCounter())
		C.dx_update_cb(r.ctx, r.cbPost, unsafe.Pointer(&u), C.int(unsafe.Sizeof(u)))
		r.bindCB(r.cbPost)
		r.bindSRV(0, input)
		if scaleMode == dxFilterMinMagMipLine {
			r.bindSampler(0, r.linearClamp)
		} else {
			r.bindSampler(0, r.pointClamp)
		}
		C.dx_draw(r.ctx, 4, 0)
	}
	if sys.cfg.Video.RendererDebugMode {
		C.dx_pull_messages(r.device)
	}
	C.dx_present(r.swapchain, C.int(r.vsyncInterval))
}

func (r *Renderer_DX) Await() {
	C.dx_flush(r.ctx)
}

func (r *Renderer_DX) bindRT(rtv, dsv unsafe.Pointer) {
	if r.state.rtv == rtv && r.state.dsv == dsv {
		return
	}
	r.state.rtv, r.state.dsv = rtv, dsv
	C.dx_set_rt(r.ctx, rtv, dsv)
}

func (r *Renderer_DX) bindViewport(x, y, w, h int32) {
	if r.state.vp == [4]int32{x, y, w, h} {
		return
	}
	r.state.vp = [4]int32{x, y, w, h}
	C.dx_set_viewport(r.ctx, C.int(x), C.int(y), C.int(w), C.int(h))
}

func (r *Renderer_DX) bindShaders(vs, ps unsafe.Pointer) {
	if r.state.vs != vs {
		r.state.vs = vs
		C.dx_set_vs(r.ctx, vs)
	}
	if r.state.ps != ps {
		r.state.ps = ps
		C.dx_set_ps(r.ctx, ps)
	}
}

func (r *Renderer_DX) bindIL(il unsafe.Pointer) {
	if r.state.il == il {
		return
	}
	r.state.il = il
	C.dx_set_il(r.ctx, il)
}

func (r *Renderer_DX) bindTopology(topo int) {
	if r.state.topo == topo {
		return
	}
	r.state.topo = topo
	C.dx_set_topology(r.ctx, C.int(topo))
}

func (r *Renderer_DX) bindVB(vb unsafe.Pointer, stride int) {
	if r.state.vb == vb && r.state.vbStride == stride {
		return
	}
	r.state.vb, r.state.vbStride = vb, stride
	C.dx_set_vb(r.ctx, vb, C.int(stride))
}

func (r *Renderer_DX) bindBlend(b unsafe.Pointer) {
	if r.state.blend == b {
		return
	}
	r.state.blend = b
	C.dx_set_blend(r.ctx, b)
}

func (r *Renderer_DX) bindDS(ds unsafe.Pointer) {
	if r.state.ds == ds {
		return
	}
	r.state.ds = ds
	C.dx_set_dsstate(r.ctx, ds)
}

func (r *Renderer_DX) bindRS(rs unsafe.Pointer) {
	if r.state.rs == rs {
		return
	}
	r.state.rs = rs
	C.dx_set_rasterizer(r.ctx, rs)
}

func (r *Renderer_DX) bindCB(cb unsafe.Pointer) {
	if r.state.cbVS != cb {
		r.state.cbVS = cb
		C.dx_set_cb_vs(r.ctx, 0, cb)
	}
	if r.state.cbPS != cb {
		r.state.cbPS = cb
		C.dx_set_cb_ps(r.ctx, 0, cb)
	}
}

func (r *Renderer_DX) bindSRV(slot int, srv unsafe.Pointer) {
	if r.state.srv[slot] == srv {
		return
	}
	r.state.srv[slot] = srv
	C.dx_set_srv_ps(r.ctx, C.int(slot), srv)
}

func (r *Renderer_DX) bindSampler(slot int, s unsafe.Pointer) {
	if r.state.sampler[slot] == s {
		return
	}
	r.state.sampler[slot] = s
	C.dx_set_sampler_ps(r.ctx, C.int(slot), s)
}

func (r *Renderer_DX) EnableScissor(x, y, width, height int32) {
	if r.state.scissorOn && r.state.scissor == [4]int32{x, y, width, height} {
		return
	}
	r.state.scissorOn = true
	r.state.scissor = [4]int32{x, y, width, height}
	x2 := Clamp(x, 0, r.width)
	y2 := Clamp(y, 0, r.height)
	w2 := Clamp(width, 0, r.width-x2)
	h2 := Clamp(height, 0, r.height-y2)
	C.dx_set_scissor(r.ctx, C.int(x2), C.int(y2), C.int(w2), C.int(h2), 1)
	r.bindRS(r.rsScissor)
}

func (r *Renderer_DX) DisableScissor() {
	if !r.state.scissorOn {
		return
	}
	r.state.scissorOn = false
	C.dx_set_scissor(r.ctx, 0, 0, 0, 0, 0)
	r.bindRS(r.rsDefault)
}

func (r *Renderer_DX) mapBlendState(eq BlendEquation, src, dst BlendFunc) unsafe.Pointer {
	key := [3]int{int(eq), int(src), int(dst)}
	if b, ok := r.blendCache[key]; ok {
		return b
	}
	d3dEq := dxBlendOpAdd
	if eq == BlendReverseSubtract {
		d3dEq = dxBlendOpRevSub
	}
	d3dSrc := dxBlendSrcAlpha
	switch src {
	case BlendOne:
		d3dSrc = dxBlendOne
	case BlendZero:
		d3dSrc = dxBlendZero
	case BlendOneMinusSrcAlpha:
		d3dSrc = dxBlendInvSrcAlpha
	case BlendDstColor:
		d3dSrc = dxBlendDstColor
	case BlendOneMinusDstColor:
		d3dSrc = dxBlendInvDstColor
	}
	d3dDst := dxBlendInvSrcAlpha
	switch dst {
	case BlendOne:
		d3dDst = dxBlendOne
	case BlendZero:
		d3dDst = dxBlendZero
	case BlendSrcAlpha:
		d3dDst = dxBlendSrcAlpha
	case BlendDstColor:
		d3dDst = dxBlendDstColor
	case BlendOneMinusDstColor:
		d3dDst = dxBlendInvDstColor
	}
	b := C.dx_create_blend(r.device, C.int(d3dEq), C.int(d3dSrc), C.int(d3dDst))
	chkRes(b)
	r.blendCache[key] = b
	return b
}

func (r *Renderer_DX) EnableBlending(eq BlendEquation, src, dst BlendFunc) {
	r.bindBlend(r.mapBlendState(eq, src, dst))
}

func (r *Renderer_DX) DisableBlending() {
	r.bindBlend(nil)
}

func (r *Renderer_DX) newTexture(width, height, depth int32, filter bool) (t Texture) {
	return r.newTextureInternal(width, height, depth, filter)
}

func (r *Renderer_DX) newPaletteTexture() (t Texture) {
	r.palLock.Lock()
	defer r.palLock.Unlock()
	slot := r.palFreeSlots.Front()
	if slot == nil {
		r.autoResizeAtlas()
		slot = r.palFreeSlots.Front()
	}
	if slot == nil {
		t = r.newTextureInternal(256, 1, 32, false)
		memPalSlotAlloc()
		return t
	}
	r.palFreeSlots.Remove(slot)
	idx := slot.Value.(int32)
	slotsPerRow := r.palAtlasSize / 256
	t = &Texture_DX{
		width:     256,
		height:    1,
		depth:     32,
		filter:    false,
		resource:  r.palAtlas.resource,
		srv:       r.palAtlas.srv,
		sampler:   r.palAtlas.sampler,
		serial:    r.palAtlas.serial,
		offsetX:   (idx % slotsPerRow) * 256,
		offsetY:   idx / slotsPerRow,
		slotIndex: idx,
		palSlot:   true,
		atlasSize: r.palAtlasSize,
	}
	runtime.SetFinalizer(t, func(t *Texture_DX) {
		if !t.palSlot {
			return
		}
		rr := gfx.(*Renderer_DX)
		rr.palLock.Lock()
		rr.palFreeSlots.PushFront(t.slotIndex)
		rr.palLock.Unlock()
	})
	memPalSlotAlloc()
	return t
}

func (r *Renderer_DX) newModelTexture(width, height, depth int32, filter bool) (t Texture) {
	return r.newTextureInternal(width, height, depth, filter)
}

func (r *Renderer_DX) newDataTexture(width, height int32) (t Texture) {
	tx := &Texture_DX{width: width, height: height, depth: 128, filter: false, serial: textureSerialNumber}
	textureSerialNumber++
	tx.resource = C.dx_create_texture(r.device, C.int(width), C.int(height), dxFmtRGBA32F, dxBindSRV, 1, 1, 1, 0, dxUsageDefault, 0)
	chkRes(tx.resource)
	tx.srv = C.dx_create_srv(r.device, tx.resource, dxFmtRGBA32F, 1, 1, 1, 0)
	chkRes(tx.srv)
	tx.sampler = r.pointClamp
	runtime.SetFinalizer(tx, func(t *Texture_DX) { t.Release() })
	return tx
}

func (r *Renderer_DX) newHDRTexture(width, height int32) (t Texture) {
	tx := &Texture_DX{width: width, height: height, depth: 128, filter: true, serial: textureSerialNumber}
	textureSerialNumber++
	tx.resource = C.dx_create_texture(r.device, C.int(width), C.int(height), dxFmtRGBA32F, dxBindSRV, 1, 1, 1, 0, dxUsageDefault, 0)
	chkRes(tx.resource)
	tx.srv = C.dx_create_srv(r.device, tx.resource, dxFmtRGBA32F, 1, 1, 1, 0)
	chkRes(tx.srv)
	tx.sampler = C.dx_create_sampler(r.device, dxFilterMinMagMipLine, dxAddrMirror, dxAddrMirror)
	chkRes(tx.sampler)
	runtime.SetFinalizer(tx, func(t *Texture_DX) { t.Release() })
	return tx
}

func (r *Renderer_DX) newCubeMapTexture(widthHeight int32, mipmap bool, lowestMipLevel int32) (t Texture) {
	mips := 1
	if mipmap {
		mips = 1
		for mips < int(widthHeight) {
			mips *= 2
		}
		mips = int(math.Log2(float64(mips))) + 1
	}
	tx := &Texture_DX{width: widthHeight, height: widthHeight, depth: 128, filter: true, serial: textureSerialNumber, isCube: true, mips: mips}
	textureSerialNumber++
	tx.resource = C.dx_create_texture(r.device, C.int(widthHeight), C.int(widthHeight), dxFmtRGBA32F, dxBindSRV, 1, C.int(mips), 6, 1, dxUsageDefault, 0)
	chkRes(tx.resource)
	tx.srv = C.dx_create_srv(r.device, tx.resource, dxFmtRGBA32F, 1, C.int(mips), 6, 1)
	chkRes(tx.srv)
	tx.sampler = r.linearClamp
	runtime.SetFinalizer(tx, func(t *Texture_DX) { t.Release() })
	return tx
}

func (r *Renderer_DX) SetVertexData(values ...float32) {
	needed := len(values) * 4
	if needed > r.vbSize {
		size := r.vbSize
		for size < needed {
			size *= 2
		}
		vb := C.dx_create_vb(r.device, C.int(size))
		chkRes(vb)
		if r.state.vb == r.vb {
			r.state.vb = nil
		}
		C.dx_release(r.vb)
		r.vb = vb
		r.vbSize = size
	}
	if cap(r.vertexScratch) < needed {
		r.vertexScratch = make([]byte, needed)
	}
	buf := r.vertexScratch[:needed]
	for i, v := range values {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	p := C.dx_map_vb(r.ctx, r.vb)
	if p == nil {
		panic("Direct3D 11: " + C.GoString(C.dx_last_error()))
	}
	copy(unsafe.Slice((*byte)(p), needed), buf)
	C.dx_unmap_vb(r.ctx, r.vb)
}

func (r *Renderer_DX) RenderQuad() {
	r.flushUniforms()
	r.bindShaders(r.vsSprite, r.currentPS())
	r.bindIL(r.ilSprite)
	r.bindTopology(dxTopoStrip)
	r.bindVB(r.vb, 16)
	C.dx_draw(r.ctx, 4, 0)
}

func (r *Renderer_DX) currentPS() unsafe.Pointer {
	if r.currentCustom != 0 {
		if sh, ok := r.customShaders[r.currentCustom]; ok && sh.ps != nil {
			return sh.ps
		}
	}
	return r.psSprite
}

func (r *Renderer_DX) flushUniforms() {
	if r.cbDirty {
		C.dx_update_cb(r.ctx, r.cb, unsafe.Pointer(&r.uniforms), C.int(unsafe.Sizeof(dxUniforms{})))
		r.cbDirty = false
	}
	r.bindCB(r.cb)
}

func (r *Renderer_DX) RenderElements(mode PrimitiveMode, count, offset int) {
	r.flushUniforms()
	r.bindTopology(r.mapPrimitiveMode(mode))
	C.dx_draw_indexed(r.ctx, C.int(count), C.int(offset))
}

func (r *Renderer_DX) mapPrimitiveMode(mode PrimitiveMode) int {
	switch mode {
	case LINES:
		return dxTopoLine
	case LINE_LOOP:
		return dxTopoLine
	case LINE_STRIP:
		return dxTopoLineStrip
	case TRIANGLES:
		return dxTopoList
	case TRIANGLE_STRIP:
		return dxTopoStrip
	case TRIANGLE_FAN:
		return dxTopoFan
	}
	return dxTopoPoints
}

func (r *Renderer_DX) SetUniformI(name string, val int) {
	r.cbDirty = true
	switch name {
	case "mask":
		r.uniforms.maskFlatRgbaTrapez[0] = int32(val)
	case "isFlat":
		r.uniforms.maskFlatRgbaTrapez[1] = int32(val)
	case "isRgba":
		r.uniforms.maskFlatRgbaTrapez[2] = int32(val)
	case "isTrapez":
		r.uniforms.maskFlatRgbaTrapez[3] = int32(val)
	case "neg":
		r.uniforms.negTime[0] = float32(val)
	}
}

func (r *Renderer_DX) SetUniformF(name string, values ...float32) {
	r.cbDirty = true
	switch name {
	case "alpha":
		r.uniforms.alphaGrayHue[0] = values[0]
	case "gray":
		r.uniforms.alphaGrayHue[1] = values[0]
	case "hue":
		r.uniforms.alphaGrayHue[2] = values[0]
	case "iTime":
		r.uniforms.negTime[1] = values[0]
	case "sTime":
		r.uniforms.negTime[2] = values[0]
	case "aspectRatio":
		r.uniforms.negTime[3] = values[0]
	case "iResolution":
		copy(r.uniforms.iResolution[:], values)
	case "palUV":
		copy(r.uniforms.palUV[:], values)
	}
}

func (r *Renderer_DX) SetUniformFv(name string, values []float32) {
	r.cbDirty = true
	switch name {
	case "tint":
		copy(r.uniforms.tint[:], values)
	case "add":
		copy(r.uniforms.add[:], values)
	case "mult":
		copy(r.uniforms.mult[:], values)
	}
}

func (r *Renderer_DX) SetUniformMatrix(name string, value []float32) {
	r.cbDirty = true
	if name == "modelview" {
		copy(r.uniforms.modelview[:], value)
	} else if name == "projection" {
		copy(r.uniforms.projection[:], value)
	}
}

func (r *Renderer_DX) SetTexture(name string, tex Texture) {
	if tex == nil {
		return
	}
	t := tex.(*Texture_DX)
	switch name {
	case "tex":
		r.bindSRV(0, t.srv)
		r.bindSampler(0, t.sampler)
	case "pal":
		r.bindSRV(1, t.srv)
		r.bindSampler(1, t.sampler)
	case "tex1":
		r.bindSRV(2, t.srv)
		r.bindSampler(2, t.sampler)
	case "tex2":
		r.bindSRV(3, t.srv)
		r.bindSampler(3, t.sampler)
	case "bgl_RenderedTexture":
		r.bindSRV(4, t.srv)
		r.bindSampler(4, t.sampler)
	}
}

func (r *Renderer_DX) LoadCustomSpriteShader(shaderName string, shaderData []byte) uint32 {
	if id, ok := r.customShaderMap[shaderName]; ok {
		return id
	}
	if len(shaderData) == 0 {
		LogMessage("[Direct3D 11] Custom shader %s has no data", shaderName)
		return 0
	}
	blob := C.dx_compile(unsafe.Pointer(&shaderData[0]), C.int(len(shaderData)), 0)
	if blob == nil {
		LogMessage("[Direct3D 11] Failed to compile custom shader %s: %s", shaderName, C.GoString(C.dx_last_error()))
		return 0
	}
	ps := C.dx_create_ps(r.device, C.dx_blob_ptr(blob), C.int(C.dx_blob_size(blob)))
	C.dx_release(blob)
	if ps == nil {
		LogMessage("[Direct3D 11] Failed to create custom shader %s: %s", shaderName, C.GoString(C.dx_last_error()))
		return 0
	}
	needsGrabPass := bytes.Contains(shaderData, []byte("bgl_RenderedTexture"))
	id := r.nextShaderID
	r.nextShaderID++
	r.customShaders[id] = &dxCustomShader{ps: ps, needsGrabPass: needsGrabPass}
	r.customShaderMap[shaderName] = id
	sys.appendToConsole(fmt.Sprintf("Loaded Custom DX Shader: %s (ID: %d, NeedsGrabPass: %v)", shaderName, id, needsGrabPass))
	return id
}

func (r *Renderer_DX) UnloadCustomSpriteShader(shaderName string) {
	if id, exists := r.customShaderMap[shaderName]; exists {
		if sh, ok := r.customShaders[id]; ok {
			if r.currentCustom == id {
				r.currentCustom = 0
			}
			C.dx_release(sh.ps)
			delete(r.customShaders, id)
		}
		delete(r.customShaderMap, shaderName)
	}
}

func (r *Renderer_DX) SetSpritePipeline(shaderName string) {
	if shaderName == "" {
		r.currentCustom = 0
		return
	}
	if id, ok := r.customShaderMap[shaderName]; ok {
		r.currentCustom = id
	}
}

func (r *Renderer_DX) SetCustomUniforms(params [16]float32) {
	r.cbDirty = true
	for i := range params {
		r.uniforms.p[i] = [4]float32{params[i], 0, 0, 0}
	}
}

func (r *Renderer_DX) NeedsGrabPass() bool {
	if r.currentCustom != 0 {
		if sh, ok := r.customShaders[r.currentCustom]; ok {
			return sh.needsGrabPass
		}
	}
	return false
}

func (r *Renderer_DX) ResolveBackBuffer() Texture {
	if r.grabTexture == nil {
		return nil
	}
	if r.rtMsaaRTV != nil {
		C.dx_resolve(r.ctx, r.grabTex, r.rtMsaa, dxFmtRGBA8)
	} else {
		C.dx_copy(r.ctx, r.grabTex, r.rtMain)
	}
	return r.grabTexture
}

func (r *Renderer_DX) ReadPixels(data []uint8, width, height int) {
	staging := C.dx_create_texture(r.device, C.int(width), C.int(height), dxFmtRGBA8, 0, 1, 1, 1, 0, dxUsageStaging, dxCPURead)
	if staging == nil {
		LogMessage("[Direct3D 11] ReadPixels staging alloc failed: %s", C.GoString(C.dx_last_error()))
		return
	}
	C.dx_copy(r.ctx, staging, r.backbuffer)
	p := C.dx_map_read(r.ctx, staging)
	if p == nil {
		LogMessage("[Direct3D 11] ReadPixels map failed: %s", C.GoString(C.dx_last_error()))
		C.dx_release(staging)
		return
	}
	src := unsafe.Slice((*byte)(p), width*height*4)
	for y := 0; y < height; y++ {
		copy(data[y*width*4:], src[(height-1-y)*width*4:])
	}
	C.dx_unmap_read(r.ctx, staging)
	C.dx_release(staging)
}

func (r *Renderer_DX) SetVSync(interval int) {
	r.vsyncInterval = interval
	if r.vsyncInterval < 0 {
		r.vsyncInterval = 1
	}
}

func (r *Renderer_DX) NewWorkerThread() bool { return false }

func (r *Renderer_DX) IsModelEnabled() bool  { return false }
func (r *Renderer_DX) IsShadowEnabled() bool { return false }

func (r *Renderer_DX) PerspectiveProjectionMatrix(angle, aspect, near, far float32) mgl.Mat4 {
	return mgl.Perspective(angle, aspect, near, far)
}

func (r *Renderer_DX) OrthographicProjectionMatrix(left, right, bottom, top, near, far float32) mgl.Mat4 {
	return mgl.Ortho(left, right, bottom, top, near, far)
}

func (r *Renderer_DX) prepareShadowMapPipeline(bufferIndex uint32) {}
func (r *Renderer_DX) setShadowMapPipeline(doubleSided, invertFrontFace, useUV, useNormal, useTangent, useVertColor, useJoint0, useJoint1 bool, numVertices, vertAttrOffset uint32) {
}
func (r *Renderer_DX) ReleaseShadowPipeline()                                    {}
func (r *Renderer_DX) prepareModelPipeline(bufferIndex uint32, env *Environment) {}
func (r *Renderer_DX) SetModelPipeline(eq BlendEquation, src, dst BlendFunc, depthTest, depthMask, doubleSided, invertFrontFace, useUV, useNormal, useTangent, useVertColor, useJoint0, useJoint1, useOutlineAttribute bool, numVertices, vertAttrOffset uint32) {
}
func (r *Renderer_DX) SetMeshOutlinePipeline(invertFrontFace bool, meshOutline float32) {}
func (r *Renderer_DX) ReleaseModelPipeline()                                            {}
func (r *Renderer_DX) RenderShadowMapElements(mode PrimitiveMode, count, offset int)    {}
func (r *Renderer_DX) RenderCubeMap(envTexture Texture, cubeTexture Texture)            {}
func (r *Renderer_DX) RenderFilteredCubeMap(distribution int32, cubeTexture Texture, filteredTexture Texture, mipmapLevel, sampleCount int32, roughness float32) {
}
func (r *Renderer_DX) RenderLUT(distribution int32, cubeTexture Texture, lutTexture Texture, sampleCount int32) {
}
func (r *Renderer_DX) SetModelUniformI(name string, val int)               {}
func (r *Renderer_DX) SetModelUniformF(name string, values ...float32)     {}
func (r *Renderer_DX) SetModelUniformFv(name string, values []float32)     {}
func (r *Renderer_DX) SetModelUniformMatrix(name string, value []float32)  {}
func (r *Renderer_DX) SetModelUniformMatrix3(name string, value []float32) {}
func (r *Renderer_DX) SetModelTexture(name string, t Texture)              {}
func (r *Renderer_DX) SetShadowMapUniformI(name string, val int)           {}
func (r *Renderer_DX) SetShadowMapUniformF(name string, values ...float32) {}
func (r *Renderer_DX) SetShadowMapUniformFv(name string, values []float32) {}
func (r *Renderer_DX) SetShadowMapUniformMatrix(name string, value []float32) {
}
func (r *Renderer_DX) SetShadowMapUniformMatrix3(name string, value []float32) {
}
func (r *Renderer_DX) SetShadowMapTexture(name string, t Texture)           {}
func (r *Renderer_DX) SetShadowFrameTexture(i uint32)                       {}
func (r *Renderer_DX) SetShadowFrameCubeTexture(i uint32)                   {}
func (r *Renderer_DX) SetModelVertexData(bufferIndex uint32, values []byte) {}
func (r *Renderer_DX) SetModelIndexData(bufferIndex uint32, values ...uint32) {
}
