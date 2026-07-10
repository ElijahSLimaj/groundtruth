import {
  Body,
  Controller,
  Get,
  Param,
  ParseUUIDPipe,
  Post,
  UseGuards,
} from '@nestjs/common';
import { IsNotEmpty, IsOptional, IsString, IsUUID } from 'class-validator';

import { InternalGuard } from '../auth/internal.guard';
import { CurrentPrincipal } from '../auth/principal';
import type { Principal } from '../auth/principal';
import { ChatService } from './chat.service';

class SendMessageDto {
  @IsOptional()
  @IsUUID()
  conversation_id?: string;

  @IsString()
  @IsNotEmpty()
  content!: string;
}

@Controller('chat')
@UseGuards(InternalGuard)
export class ChatController {
  constructor(private readonly chat: ChatService) {}

  @Get('conversations')
  listConversations(@CurrentPrincipal() principal: Principal) {
    return this.chat.listConversations(principal);
  }

  @Get('conversations/:id')
  getConversation(
    @CurrentPrincipal() principal: Principal,
    @Param('id', new ParseUUIDPipe({ errorHttpStatusCode: 404 })) id: string,
  ) {
    return this.chat.getConversation(principal, id);
  }

  @Post('messages')
  sendMessage(
    @CurrentPrincipal() principal: Principal,
    @Body() dto: SendMessageDto,
  ) {
    return this.chat.sendMessage(
      principal,
      dto.conversation_id ?? null,
      dto.content,
    );
  }
}
