import { en as quickConnectMessages } from '@/custom/vote-ai/quick-connect/messages'
import { startPageMessages } from '@/custom/vote-ai/quick-connect/page-messages'
import { en as quickCreateKey } from '@/custom/vote-ai/quick-connect/create-key-messages'
import { entryMessages } from '@/custom/vote-ai/quick-connect/entry-messages'
import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import channelMonitorV2 from './channelMonitorV2'
import batchImage from './batchImage'
import admin from './admin'
import misc from './misc'

export default {
  rikkaHub: rikkaHubMessages.en,
  quickConnect: { ...quickConnectMessages, startPage: startPageMessages.en, entryFlow: entryMessages.en },
  quickCreateKey,
  ...landing,
  ...common,
  ...dashboard,
  ...channelMonitorV2,
  ...batchImage,
  admin,
  ...misc,
}
import rikkaHubMessages from '@/custom/vote-ai/rikkahub/messages'
