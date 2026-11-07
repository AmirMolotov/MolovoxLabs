// Offline host integration fixture, not a user-facing effect.
#define NOMINMAX
#define INIT_CLASS_IID
#include <windows.h>
#include "pluginterfaces/base/ipluginbase.h"
#include "pluginterfaces/base/ibstream.h"
#include "pluginterfaces/vst/ivstaudioprocessor.h"
#include "pluginterfaces/vst/ivsteditcontroller.h"
#include "pluginterfaces/vst/ivstparameterchanges.h"
#include "pluginterfaces/vst/vstspeaker.h"
#include <atomic>
#include <cstring>
#include <cstdio>
using namespace Steinberg;using namespace Steinberg::Vst;
static const TUID classID=INLINE_UID(0x841E83E1,0x122B4701,0x9E337FA0,0x6B392140);
template<class T>bool is(const TUID id){return FUnknownPrivate::iidEqual(id,T::iid);}
#define REFS std::atomic<uint32> refs{1};uint32 PLUGIN_API addRef()override{return ++refs;}uint32 PLUGIN_API release()override{auto n=--refs;if(!n)delete this;return n;}
class Gain:public IComponent,public IAudioProcessor,public IEditController {
 std::atomic<double> gain{.5};
public:REFS
 tresult PLUGIN_API queryInterface(const TUID id,void** p)override{*p=nullptr;if(is<FUnknown>(id)||is<IComponent>(id))*p=(IComponent*)this;else if(is<IAudioProcessor>(id))*p=(IAudioProcessor*)this;else if(is<IEditController>(id))*p=(IEditController*)this;else return kNoInterface;addRef();return kResultOk;}
 tresult PLUGIN_API initialize(FUnknown*)override{return kResultOk;}
 tresult PLUGIN_API terminate()override{return kResultOk;}
 tresult PLUGIN_API getControllerClassId(TUID)override{return kResultFalse;}
 tresult PLUGIN_API setIoMode(IoMode)override{return kResultOk;}
 int32 PLUGIN_API getBusCount(MediaType media,BusDirection)override{return media==kAudio?1:0;}
 tresult PLUGIN_API getBusInfo(MediaType media,BusDirection dir,int32 index,BusInfo& info)override{if(media!=kAudio||index)return kResultFalse;memset(&info,0,sizeof(info));info.mediaType=media;info.direction=dir;info.channelCount=2;info.busType=kMain;info.flags=BusInfo::kDefaultActive;return kResultOk;}
 tresult PLUGIN_API getRoutingInfo(RoutingInfo&,RoutingInfo&)override{return kNotImplemented;}
 tresult PLUGIN_API activateBus(MediaType,BusDirection,int32,TBool)override{return kResultOk;}
 tresult PLUGIN_API setActive(TBool)override{return kResultOk;}
 tresult PLUGIN_API setState(IBStream* s)override{double v=.5;int32 n=0;if(s->read(&v,8,&n)!=kResultOk||n!=8)return kResultFalse;gain=v;return kResultOk;}
 tresult PLUGIN_API getState(IBStream* s)override{double v=gain.load();return s->write(&v,8);}
 tresult PLUGIN_API setBusArrangements(SpeakerArrangement*,int32 ni,SpeakerArrangement*,int32 no)override{return ni==1&&no==1?kResultOk:kResultFalse;}
 tresult PLUGIN_API getBusArrangement(BusDirection,int32 index,SpeakerArrangement& a)override{a=SpeakerArr::kStereo;return index? kResultFalse:kResultOk;}
 tresult PLUGIN_API canProcessSampleSize(int32 size)override{return size==kSample32?kResultOk:kResultFalse;}
 uint32 PLUGIN_API getLatencySamples()override{return 0;}
 tresult PLUGIN_API setupProcessing(ProcessSetup& s)override{return s.sampleRate==48000&&s.maxSamplesPerBlock>=128?kResultOk:kResultFalse;}
 tresult PLUGIN_API setProcessing(TBool)override{return kResultOk;}
 tresult PLUGIN_API process(ProcessData& d)override{if(d.inputParameterChanges){for(int i=0;i<d.inputParameterChanges->getParameterCount();i++){auto q=d.inputParameterChanges->getParameterData(i);if(q&&q->getParameterId()==100){int32 offset;double value;if(q->getPoint(0,offset,value)==kResultOk)gain=value;}}}double v=gain.load();for(int c=0;c<2;c++)for(int i=0;i<d.numSamples;i++)d.outputs[0].channelBuffers32[c][i]=(float)(d.inputs[0].channelBuffers32[c][i]*v);return kResultOk;}
 uint32 PLUGIN_API getTailSamples()override{return 0;}
 tresult PLUGIN_API setComponentState(IBStream* s)override{return setState(s);}
 int32 PLUGIN_API getParameterCount()override{return 1;}
 tresult PLUGIN_API getParameterInfo(int32 index,ParameterInfo& info)override{if(index)return kResultFalse;memset(&info,0,sizeof(info));info.id=100;const wchar_t* name=L"Test gain";memcpy(info.title,name,(wcslen(name)+1)*2);info.defaultNormalizedValue=.5;info.flags=ParameterInfo::kCanAutomate;return kResultOk;}
 tresult PLUGIN_API getParamStringByValue(ParamID,ParamValue v,String128 text)override{swprintf_s((wchar_t*)text,128,L"%.1f %%",v*100);return kResultOk;}
 tresult PLUGIN_API getParamValueByString(ParamID,TChar*,ParamValue&)override{return kNotImplemented;}
 ParamValue PLUGIN_API normalizedParamToPlain(ParamID,ParamValue v)override{return v;}
 ParamValue PLUGIN_API plainParamToNormalized(ParamID,ParamValue v)override{return v;}
 ParamValue PLUGIN_API getParamNormalized(ParamID)override{return gain.load();}
 tresult PLUGIN_API setParamNormalized(ParamID,ParamValue v)override{gain=v;return kResultOk;}
 tresult PLUGIN_API setComponentHandler(IComponentHandler*)override{return kResultOk;}
 IPlugView* PLUGIN_API createView(FIDString)override{return nullptr;}
};
class Factory:public IPluginFactory2 {
public:REFS
 tresult PLUGIN_API queryInterface(const TUID id,void** p)override{*p=nullptr;if(is<FUnknown>(id)||is<IPluginFactory>(id)||is<IPluginFactory2>(id)){*p=(IPluginFactory2*)this;addRef();return kResultOk;}return kNoInterface;}
 tresult PLUGIN_API getFactoryInfo(PFactoryInfo* info)override{memset(info,0,sizeof(*info));strcpy_s(info->vendor,"Molovox Labs Tests");return kResultOk;}
 int32 PLUGIN_API countClasses()override{return 1;}
 tresult PLUGIN_API getClassInfo(int32 index,PClassInfo* info)override{if(index)return kResultFalse;memset(info,0,sizeof(*info));memcpy(info->cid,classID,16);info->cardinality=PClassInfo::kManyInstances;strcpy_s(info->category,kVstAudioEffectClass);strcpy_s(info->name,"Molovox Labs Test Gain");return kResultOk;}
 tresult PLUGIN_API getClassInfo2(int32 index,PClassInfo2* info)override{if(index)return kResultFalse;memset(info,0,sizeof(*info));memcpy(info->cid,classID,16);info->cardinality=PClassInfo::kManyInstances;strcpy_s(info->category,kVstAudioEffectClass);strcpy_s(info->name,"Molovox Labs Test Gain");strcpy_s(info->subCategories,"Fx");return kResultOk;}
 tresult PLUGIN_API createInstance(FIDString cid,FIDString iid,void** p)override{*p=nullptr;if(memcmp(cid,classID,16))return kResultFalse;auto gain=new Gain;auto r=gain->queryInterface(iid,p);gain->release();return r;}
};
extern "C" __declspec(dllexport) IPluginFactory* GetPluginFactory(){return new Factory;}
extern "C" __declspec(dllexport) bool InitDll(){return true;}
extern "C" __declspec(dllexport) bool ExitDll(){return true;}
