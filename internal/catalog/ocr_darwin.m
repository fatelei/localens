//go:build darwin && cgo

#import <Foundation/Foundation.h>
#import <Vision/Vision.h>
#import <ImageIO/ImageIO.h>
#include <stdlib.h>
#include <string.h>

char *localens_recognize_text(const char *path, char **failure) {
 @autoreleasepool {
  NSURL *url=[NSURL fileURLWithPath:[NSString stringWithUTF8String:path]];
  CGImageSourceRef source=CGImageSourceCreateWithURL((__bridge CFURLRef)url,NULL);
  if(!source){*failure=strdup("Image could not be decoded");return NULL;}
  CFDictionaryRef rawProperties=CGImageSourceCopyPropertiesAtIndex(source,0,NULL);NSDictionary *properties=(__bridge NSDictionary *)rawProperties;
  double width=[properties[(id)kCGImagePropertyPixelWidth] doubleValue],height=[properties[(id)kCGImagePropertyPixelHeight] doubleValue];
  if(rawProperties)CFRelease(rawProperties);
  if(width<=0||height<=0||width>20000||height>20000||width*height>80000000){CFRelease(source);*failure=strdup("Image dimensions exceed OCR limits");return NULL;}
  NSDictionary *options=@{(id)kCGImageSourceCreateThumbnailFromImageAlways:@YES,(id)kCGImageSourceThumbnailMaxPixelSize:@2400,(id)kCGImageSourceCreateThumbnailWithTransform:@YES,(id)kCGImageSourceShouldCacheImmediately:@YES};
  CGImageRef image=CGImageSourceCreateThumbnailAtIndex(source,0,(__bridge CFDictionaryRef)options);
  CFRelease(source);
  if(!image){*failure=strdup("Image thumbnail could not be decoded");return NULL;}
  VNRecognizeTextRequest *request=[[VNRecognizeTextRequest alloc]init];
  request.recognitionLevel=VNRequestTextRecognitionLevelAccurate;
  request.usesLanguageCorrection=YES;
  NSError *error=nil;
  NSArray *supported=[request supportedRecognitionLanguagesAndReturnError:&error];
  NSMutableArray *languages=[NSMutableArray array];
  for(NSString *language in @[@"zh-Hans",@"zh-Hant",@"en-US"]){if([supported containsObject:language])[languages addObject:language];}
  if(languages.count)request.recognitionLanguages=languages;
  VNImageRequestHandler *handler=[[VNImageRequestHandler alloc]initWithCGImage:image options:@{}];
  BOOL success=[handler performRequests:@[request] error:&error];CGImageRelease(image);
  char *result=NULL;
  if(!success){*failure=strdup([[NSString stringWithFormat:@"Offline text recognition failed (%@:%ld)",error.domain,(long)error.code] UTF8String]);}
  else {
   NSMutableString *text=[NSMutableString string];
   for(VNRecognizedTextObservation *observation in request.results){
    VNRecognizedText *candidate=[[observation topCandidates:1] firstObject];
    if(candidate&&candidate.confidence>=0.2){[text appendString:candidate.string];[text appendString:@"\n"];}
    if([text length]>131072)break;
   }
   result=strdup(text.UTF8String);
  }
#if !__has_feature(objc_arc)
  [handler release];[request release];
#endif
  return result;
 }
}
